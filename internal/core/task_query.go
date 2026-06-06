package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	taskIDParentRE = regexp.MustCompile(`^[A-Z]+-[0-9]+$`)
	taskIDChildRE  = regexp.MustCompile(`^[A-Z]+-[0-9]+-[a-z]$`)
)

func ValidateTaskID(id string) error {
	if taskIDParentRE.MatchString(id) || taskIDChildRE.MatchString(id) {
		return nil
	}
	return fmt.Errorf("invalid task ID format: %q", id)
}

type TaskListOptions struct {
	ProjectRoot string
	Location    string // optional: inbox|active|done|failed
	Query       string // optional text search in id/summary/goal
	ParentTask  string // optional
	Limit       int
	Offset      int
}

type TaskListResponse struct {
	RootPath string             `json:"root_path"`
	Counts   TaskLocationCounts `json:"counts"`
	Items    []TaskListItem     `json:"items"`
}

type TaskLocationCounts struct {
	Inbox  int `json:"inbox"`
	Active int `json:"active"`
	Done   int `json:"done"`
	Failed int `json:"failed"`
}

type TaskListItem struct {
	ID              string         `json:"id"`
	Directory       string         `json:"directory"`
	Location        string         `json:"location"`
	Summary         string         `json:"summary"`
	Goal            string         `json:"goal,omitempty"`
	Risk            string         `json:"risk,omitempty"`
	ParentTask      string         `json:"parent_task,omitempty"`
	ParentState     string         `json:"parent_state,omitempty"`
	Children        []TaskChildRef `json:"children,omitempty"`
	Artifacts       map[string]bool `json:"artifacts"`
	WorktreePresent bool           `json:"worktree_present"`
	UpdatedAt       string         `json:"updated_at"`
}

type TaskChildRef struct {
	ID       string `json:"id"`
	Location string `json:"location"`
	Summary  string `json:"summary,omitempty"`
}

type TaskDetail struct {
	TaskListItem
	Spec              map[string]any       `json:"spec,omitempty"`
	Blueprint         map[string]any       `json:"blueprint,omitempty"`
	Contract          TaskContractSummary  `json:"contract,omitempty"`
	ImplementationLog map[string]any       `json:"implementation_log,omitempty"`
	Validation        map[string]any       `json:"validation,omitempty"`
	Review            map[string]any       `json:"review,omitempty"`
	Trace             TaskTraceSummary     `json:"trace,omitempty"`
	Feedback          map[string]any       `json:"feedback,omitempty"`
}

type TaskContractSummary struct {
	Summary        string   `json:"summary"`
	Goal           string   `json:"goal"`
	Touch          []string `json:"touch"`
	VerifyCommands []string `json:"verify_commands"`
}

type TaskTraceSummary struct {
	Summary       string         `json:"summary"`
	AttemptsCount int            `json:"attempts_count"`
	LastAttempt   map[string]any `json:"last_attempt,omitempty"`
	TotalCostUSD  float64        `json:"total_cost_usd,omitempty"`
}

func QueryTasks(opts TaskListOptions) (*TaskListResponse, error) {
	if opts.ProjectRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}

	locations := []string{"inbox", "active", "done", "failed"}
	counts := TaskLocationCounts{}

	for _, loc := range locations {
		locPath := filepath.Join(opts.ProjectRoot, ".ai", "tasks", loc)
		entries, err := os.ReadDir(locPath)
		if err != nil {
			continue
		}
		count := 0
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() != "_template" {
				count++
			}
		}
		switch loc {
		case "inbox":
			counts.Inbox = count
		case "active":
			counts.Active = count
		case "done":
			counts.Done = count
		case "failed":
			counts.Failed = count
		}
	}

	var scanLocations []string
	if opts.Location != "" {
		if opts.Location != "inbox" && opts.Location != "active" && opts.Location != "done" && opts.Location != "failed" {
			return nil, fmt.Errorf("invalid location: %q", opts.Location)
		}
		scanLocations = []string{opts.Location}
	} else {
		scanLocations = locations
	}

	var items []TaskListItem
	for _, loc := range scanLocations {
		locPath := filepath.Join(opts.ProjectRoot, ".ai", "tasks", loc)
		entries, err := os.ReadDir(locPath)
		if err != nil {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "_template" {
				continue
			}
			item, err := loadTaskListItem(opts.ProjectRoot, loc, entry.Name())
			if err != nil {
				item = TaskListItem{
					Directory: entry.Name(),
					Location:  loc,
					Summary:   "<unreadable task>",
					Artifacts: make(map[string]bool),
				}
			}

			if opts.ParentTask != "" && item.ParentTask != opts.ParentTask {
				continue
			}

			if opts.Query != "" {
				q := strings.ToLower(opts.Query)
				idMatch := strings.Contains(strings.ToLower(item.ID), q)
				sumMatch := strings.Contains(strings.ToLower(item.Summary), q)
				goalMatch := strings.Contains(strings.ToLower(item.Goal), q)
				dirMatch := strings.Contains(strings.ToLower(item.Directory), q)
				if !idMatch && !sumMatch && !goalMatch && !dirMatch {
					continue
				}
			}

			items = append(items, item)
		}
	}

	totalItems := len(items)
	start := opts.Offset
	if start < 0 {
		start = 0
	}
	if start > totalItems {
		start = totalItems
	}
	end := totalItems
	if opts.Limit > 0 {
		end = start + opts.Limit
		if end > totalItems {
			end = totalItems
		}
	}

	paginated := items[start:end]
	if paginated == nil {
		paginated = []TaskListItem{}
	}

	return &TaskListResponse{
		RootPath: opts.ProjectRoot,
		Counts:   counts,
		Items:    paginated,
	}, nil
}

func GetTaskDetailIn(projectRoot, taskID string) (*TaskDetail, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return nil, err
	}
	m, err := FindTaskDirIn(projectRoot, taskID, nil)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}

	// Double check path safety to prevent traversal outside .ai/tasks
	tasksDir := filepath.Join(projectRoot, ".ai", "tasks")
	rel, err := filepath.Rel(tasksDir, m.Path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("path traversal attempt detected")
	}

	item, err := loadTaskListItem(projectRoot, m.Location, filepath.Base(m.Path))
	if err != nil {
		return nil, err
	}

	detail := &TaskDetail{
		TaskListItem: item,
	}

	dir := m.Path

	// 00-spec.yaml
	if payload, err := LoadArtifactPayload(filepath.Join(dir, "00-spec.yaml")); err == nil {
		if m, ok := payload.(map[string]any); ok {
			detail.Spec = m
		}
	}

	// 01-blueprint.yaml
	if payload, err := LoadArtifactPayload(filepath.Join(dir, "01-blueprint.yaml")); err == nil {
		if m, ok := payload.(map[string]any); ok {
			detail.Blueprint = m
		}
	}

	// 02-contract.yaml (summarized)
	var contractSummary TaskContractSummary
	contractSummary.Touch = []string{}
	contractSummary.VerifyCommands = []string{}
	if cPayload, err := LoadArtifactPayload(filepath.Join(dir, "02-contract.yaml")); err == nil {
		if cMap, ok := cPayload.(map[string]any); ok {
			if s, ok := cMap["summary"].(string); ok {
				contractSummary.Summary = s
			}
			if g, ok := cMap["goal"].(string); ok {
				contractSummary.Goal = g
			}
			if tList, ok := asSlice(cMap["touch"]); ok {
				for _, t := range tList {
					if s, ok := t.(string); ok {
						contractSummary.Touch = append(contractSummary.Touch, s)
					}
				}
			}
			if vObj, ok := cMap["verify"].(map[string]any); ok {
				if cList, ok := asSlice(vObj["commands"]); ok {
					for _, c := range cList {
						if s, ok := c.(string); ok {
							contractSummary.VerifyCommands = append(contractSummary.VerifyCommands, s)
						}
					}
				}
			}
		}
	}
	detail.Contract = contractSummary

	// 04-implementation-log.yaml
	if payload, err := LoadArtifactPayload(filepath.Join(dir, "04-implementation-log.yaml")); err == nil {
		if m, ok := payload.(map[string]any); ok {
			detail.ImplementationLog = m
		}
	}

	// 05-validation.json
	if payload, err := LoadArtifactPayload(filepath.Join(dir, "05-validation.json")); err == nil {
		if m, ok := payload.(map[string]any); ok {
			detail.Validation = m
		}
	}

	// 06-review.json
	if payload, err := LoadArtifactPayload(filepath.Join(dir, "06-review.json")); err == nil {
		if m, ok := payload.(map[string]any); ok {
			detail.Review = m
		}
	}

	// 07-trace.json (summarized)
	var traceSummary TaskTraceSummary
	if tPayload, err := LoadArtifactPayload(filepath.Join(dir, "07-trace.json")); err == nil {
		if tMap, ok := tPayload.(map[string]any); ok {
			if s, ok := tMap["summary"].(string); ok {
				traceSummary.Summary = s
			}
			if cost, ok := tMap["total_cost_usd"].(float64); ok {
				traceSummary.TotalCostUSD = cost
			} else if cost, ok := tMap["total_cost_usd"].(int); ok {
				traceSummary.TotalCostUSD = float64(cost)
			}
			if attList, ok := asSlice(tMap["attempts"]); ok {
				traceSummary.AttemptsCount = len(attList)
				if len(attList) > 0 {
					if last, ok := attList[len(attList)-1].(map[string]any); ok {
						traceSummary.LastAttempt = last
					}
				}
			}
		}
	}
	detail.Trace = traceSummary

	// feedback.json
	if payload, err := LoadArtifactPayload(filepath.Join(dir, "feedback.json")); err == nil {
		if m, ok := payload.(map[string]any); ok {
			detail.Feedback = m
		}
	}

	return detail, nil
}

func loadTaskListItem(projectRoot, location, dirName string) (TaskListItem, error) {
	dir := filepath.Join(projectRoot, ".ai", "tasks", location, dirName)

	id, _ := readSpecTaskID(dir)
	if id == "" {
		parts := strings.Split(dirName, "-")
		if len(parts) >= 3 && len(parts[2]) == 1 && parts[2] >= "a" && parts[2] <= "z" {
			id = strings.Join(parts[:3], "-")
		} else if len(parts) >= 2 {
			id = strings.Join(parts[:2], "-")
		} else {
			id = dirName
		}
	}

	artifacts := map[string]bool{
		"00-spec.yaml":                false,
		"01-blueprint.yaml":           false,
		"02-contract.yaml":            false,
		"04-implementation-log.yaml":  false,
		"05-validation.json":          false,
		"06-review.json":              false,
		"07-trace.json":               false,
		"feedback.json":               false,
	}
	for name := range artifacts {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			artifacts[name] = true
		}
	}

	var lastMod time.Time
	if info, err := os.Stat(dir); err == nil {
		lastMod = info.ModTime()
	}
	for name := range artifacts {
		if artifacts[name] {
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
				if info.ModTime().After(lastMod) {
					lastMod = info.ModTime()
				}
			}
		}
	}
	updatedAt := lastMod.UTC().Format(time.RFC3339)

	var summary, goal, risk, parentTask, parentState string
	var children []TaskChildRef
	var spec map[string]any

	if specPayload, err := LoadArtifactPayload(filepath.Join(dir, "00-spec.yaml")); err == nil {
		if m, ok := specPayload.(map[string]any); ok {
			spec = m
			if s, ok := m["summary"].(string); ok {
				summary = s
			}
			if g, ok := m["goal"].(string); ok {
				goal = g
			}
			if r, ok := m["risk"].(string); ok {
				risk = r
			}
			if pt, ok := m["parent_task"].(string); ok {
				parentTask = pt
			}
		}
	}

	if summary == "" {
		for _, name := range []string{"01-blueprint.yaml", "02-contract.yaml", "07-trace.json"} {
			if artifacts[name] {
				if payload, err := LoadArtifactPayload(filepath.Join(dir, name)); err == nil {
					if m, ok := payload.(map[string]any); ok {
						if s, ok := m["summary"].(string); ok && s != "" {
							summary = s
							break
						}
					}
				}
			}
		}
	}

	if goal == "" {
		for _, name := range []string{"01-blueprint.yaml", "02-contract.yaml"} {
			if artifacts[name] {
				if payload, err := LoadArtifactPayload(filepath.Join(dir, name)); err == nil {
					if m, ok := payload.(map[string]any); ok {
						if g, ok := m["goal"].(string); ok && g != "" {
							goal = g
							break
						}
					}
				}
			}
		}
	}

	if spec != nil {
		if spec["decomposition"] != nil {
			parentState = DeriveParentStateIn(projectRoot, spec)

			if decomp, ok := spec["decomposition"].([]any); ok {
				for _, entryAny := range decomp {
					if entry, ok := entryAny.(map[string]any); ok {
						if childID, ok := entry["child_id"].(string); ok && childID != "" {
							childLoc := "inbox"
							childSummary := ""
							if c, err := FindTaskDirIn(projectRoot, childID, nil); err == nil && c != nil {
								childLoc = c.Location
								if cSpecPayload, err := LoadArtifactPayload(filepath.Join(c.Path, "00-spec.yaml")); err == nil {
									if cSpec, ok := cSpecPayload.(map[string]any); ok {
										if s, ok := cSpec["summary"].(string); ok {
											childSummary = s
										}
									}
								}
							}
							children = append(children, TaskChildRef{
								ID:       childID,
								Location: childLoc,
								Summary:  childSummary,
							})
						}
					}
				}
			}
		}
	}

	worktreePath := filepath.Join(projectRoot, "worktrees", id)
	worktreePresent := false
	if _, err := os.Stat(worktreePath); err == nil {
		worktreePresent = true
	}

	return TaskListItem{
		ID:              id,
		Directory:       dirName,
		Location:        location,
		Summary:         summary,
		Goal:            goal,
		Risk:            risk,
		ParentTask:      parentTask,
		ParentState:     parentState,
		Children:        children,
		Artifacts:       artifacts,
		WorktreePresent: worktreePresent,
		UpdatedAt:       updatedAt,
	}, nil
}
