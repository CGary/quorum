package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"quorum/internal/core"
)

// fleetRouteRequest is the stdin Value Object for `quorum fleet route`. TaskID
// and DispatchID are CLI-only fields with no core.RouteRequest equivalent:
// TaskID gates the optional trace append, DispatchID is an opt passthrough
// correlator (empty when the request predates a minted dispatch id). Control is
// deliberately absent — it is read from .ai/fleet-control.json, never supplied
// by the caller.
type fleetRouteRequest struct {
	TaskID          string           `json:"task_id"`
	Phase           string           `json:"phase"`
	Risk            string           `json:"risk"`
	ComplexityBand  string           `json:"complexity_band"`
	IncumbentFamily string           `json:"incumbent_family,omitempty"`
	Exclusions      []core.Candidate `json:"exclusions,omitempty"`
	DispatchID      string           `json:"dispatch_id,omitempty"`
}

// fleetRouteAgentsFile mirrors the additive slice of .agents/fleet/agents.yaml
// this shim needs: per-transport active flag and per-model provider. It reads
// ONLY the fields it uses (model_arg/reasoning_effort/etc. are ignored here);
// the full transport contract stays owned by cmd/fleet_dispatch.go.
type fleetRouteAgentsFile struct {
	Transports map[string]struct {
		Active bool `yaml:"active"`
		Models map[string]struct {
			Provider string `yaml:"provider"`
		} `yaml:"models"`
	} `yaml:"transports"`
}

// fleetRouteConfigFile mirrors the slice of .agents/config.yaml this shim
// parses: the level->models map and the explicit transport-ordering list that
// expresses cross-provider reroute preference as data, not Go logic.
type fleetRouteConfigFile struct {
	Levels   map[int]core.LevelModels `yaml:"levels"`
	Policies struct {
		FleetTransportOrder []string `yaml:"fleet_transport_order"`
	} `yaml:"policies"`
}

// fleetRouteRoutingFile mirrors the `rules:` list of routing.yaml; legacy keys
// (reviewer_required/human_gate_required/type_overrides/routes) are silently
// ignored by yaml.Unmarshal into core.RoutingRule.
type fleetRouteRoutingFile struct {
	Rules []core.RoutingRule `yaml:"rules"`
}

var fleetRouteCmd = &cobra.Command{
	Use:   "route",
	Short: "Resolve an executor candidate from policy (reads a JSON request from stdin)",
	Long: `quorum fleet route reads a JSON routing request from stdin, loads and
sha256-hashes the three policy files (.agents/config.yaml,
.agents/policies/routing.yaml, .agents/fleet/agents.yaml), reads
.ai/fleet-control.json if present, calls the pure core.Route, and prints the
RouteResult JSON on stdout. When the request carries a task_id it also appends
one routing_decision event to that task's 07-trace.json via the existing
append-only SaveArtifact path. It hardcodes zero model or agent names.`,
	Run: func(cmd *cobra.Command, args []string) {
		root, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] cannot resolve project root:", err)
			os.Exit(1)
		}
		if code := runFleetRoute(root, os.Stdin, os.Stdout, os.Stderr); code != 0 {
			os.Exit(code)
		}
	},
}

// runFleetRoute is the testable core: it reads a request from stdin, composes a
// RoutePolicy/RouteRequest strictly from on-disk policy files, calls core.Route,
// optionally appends a routing_decision trace event, prints the RouteResult to
// stdout, and returns a process exit code. It never panics: every malformed
// input yields a clear stderr message and a non-zero return.
func runFleetRoute(projectRoot string, stdin io.Reader, stdout, stderr io.Writer) int {
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "[!] "+format+"\n", a...)
		return 1
	}

	raw, err := io.ReadAll(stdin)
	if err != nil {
		return fail("cannot read stdin: %v", err)
	}
	if len(raw) == 0 {
		return fail("empty stdin: expected a JSON routing request")
	}
	var req fleetRouteRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return fail("cannot parse routing request: %v", err)
	}

	policy, err := buildRoutePolicyFromDisk(projectRoot)
	if err != nil {
		return fail("%v", err)
	}

	control, err := loadFleetControlState(projectRoot)
	if err != nil {
		return fail("%v", err)
	}

	result, err := core.Route(core.RouteRequest{
		Phase:           req.Phase,
		Risk:            req.Risk,
		ComplexityBand:  req.ComplexityBand,
		IncumbentFamily: req.IncumbentFamily,
		Control:         control,
		Exclusions:      req.Exclusions,
	}, policy)
	if err != nil {
		return fail("routing failed: %v", err)
	}

	if req.TaskID != "" {
		if err := appendRoutingDecisionEvent(projectRoot, req, result); err != nil {
			return fail("%v", err)
		}
	}

	out, err := json.Marshal(result)
	if err != nil {
		return fail("cannot marshal route result: %v", err)
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

// buildRoutePolicyFromDisk reads the three policy files relative to projectRoot,
// computes their content hashes, and assembles a RoutePolicy. All file I/O lives
// here so core.Route stays pure. The transport slice is built by walking
// config.yaml's fleet_transport_order (a transport absent from that list is
// excluded, never appended), so transport ordering — the cross-provider reroute
// preference — is policy data, not Go logic.
func buildRoutePolicyFromDisk(projectRoot string) (core.RoutePolicy, error) {
	configPath := filepath.Join(projectRoot, ".agents", "config.yaml")
	routingPath := filepath.Join(projectRoot, ".agents", "policies", "routing.yaml")
	agentsPath := fleetAgentsPath(projectRoot)

	configRaw, err := os.ReadFile(configPath)
	if err != nil {
		return core.RoutePolicy{}, fmt.Errorf("cannot read %s: %w", configPath, err)
	}
	routingRaw, err := os.ReadFile(routingPath)
	if err != nil {
		return core.RoutePolicy{}, fmt.Errorf("cannot read %s: %w", routingPath, err)
	}
	agentsRaw, err := os.ReadFile(agentsPath)
	if err != nil {
		return core.RoutePolicy{}, fmt.Errorf("cannot read %s: %w", agentsPath, err)
	}

	var config fleetRouteConfigFile
	if err := yaml.Unmarshal(configRaw, &config); err != nil {
		return core.RoutePolicy{}, fmt.Errorf("cannot parse %s: %w", configPath, err)
	}
	var routing fleetRouteRoutingFile
	if err := yaml.Unmarshal(routingRaw, &routing); err != nil {
		return core.RoutePolicy{}, fmt.Errorf("cannot parse %s: %w", routingPath, err)
	}
	var agents fleetRouteAgentsFile
	if err := yaml.Unmarshal(agentsRaw, &agents); err != nil {
		return core.RoutePolicy{}, fmt.Errorf("cannot parse %s: %w", agentsPath, err)
	}

	transports := make([]core.TransportAvailability, 0, len(config.Policies.FleetTransportOrder))
	for _, name := range config.Policies.FleetTransportOrder {
		t, ok := agents.Transports[name]
		if !ok {
			continue // absent from agents.yaml -> unreachable via fleet route
		}
		ta := core.TransportAvailability{Agent: name, Active: t.Active}
		for model, m := range t.Models {
			ta.Models = append(ta.Models, core.ModelAvailability{Model: model, Family: m.Provider})
		}
		transports = append(transports, ta)
	}

	return core.RoutePolicy{
		Routing:    routing.Rules,
		Levels:     config.Levels,
		Transports: transports,
		Hashes: core.PolicyHashes{
			ConfigYAML:  hashBytes(configRaw),
			RoutingYAML: hashBytes(routingRaw),
			AgentsYAML:  hashBytes(agentsRaw),
		},
	}, nil
}

// hashBytes returns the sha256 hex digest of raw, mirroring the bundle hashing
// pattern (internal/core/fleet_bundle.go).
func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// loadFleetControlState reads <projectRoot>/.ai/fleet-control.json (doc 11). A
// missing file yields an empty ControlState (runtime state, gitignored, absent
// in a fresh repo) — never an error. A present-but-unparseable file is a real
// operational problem and fails fast.
func loadFleetControlState(projectRoot string) (core.ControlState, error) {
	path := filepath.Join(projectRoot, ".ai", "fleet-control.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return core.ControlState{}, nil
		}
		return core.ControlState{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var control core.ControlState
	if err := json.Unmarshal(raw, &control); err != nil {
		return core.ControlState{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return control, nil
}

// appendRoutingDecisionEvent resolves the active task, loads its 07-trace.json,
// appends one routing_decision event carrying the full inputs_snapshot, and
// persists through the append-only SaveArtifact path (never a direct write).
func appendRoutingDecisionEvent(projectRoot string, req fleetRouteRequest, result core.RouteResult) error {
	store := core.NewTaskStore(projectRoot)
	taskDir, err := store.FindTask(req.TaskID, "active")
	if err != nil {
		return fmt.Errorf("cannot resolve active task %s: %w", req.TaskID, err)
	}
	if taskDir == nil {
		return fmt.Errorf("active task %s not found", req.TaskID)
	}

	payload, err := store.LoadArtifact(taskDir, "07-trace.json")
	if err != nil {
		return fmt.Errorf("cannot load 07-trace.json for %s: %w", req.TaskID, err)
	}
	trace, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("07-trace.json for %s is not a JSON object", req.TaskID)
	}

	event := map[string]any{
		"type":                   "routing_decision",
		"ts":                     time.Now().UTC().Format(time.RFC3339),
		"dispatch_id":            req.DispatchID,
		"candidate":              result.Candidate,
		"blocked":                result.Blocked,
		"reasons":                result.Reasons,
		"reroute_budget":         result.RerouteBudget,
		"review_family_degraded": result.ReviewFamilyDegraded,
		"inputs_snapshot":        result.InputsSnapshot,
	}
	// Round-trip the event to a pure generic JSON value (map/slice/scalar) so it
	// matches the shape LoadArtifactPayload produces and the schema validator
	// (santhosh-tekuri) expects; a map holding Go structs/pointers would not
	// validate or compare correctly against the on-disk events.
	genericEvent, err := toGenericJSON(event)
	if err != nil {
		return fmt.Errorf("cannot normalize routing_decision event: %w", err)
	}

	events, _ := trace["events"].([]any)
	trace["events"] = append(events, genericEvent)

	if _, err := store.SaveArtifact(taskDir, "07-trace.json", trace); err != nil {
		return fmt.Errorf("cannot append routing_decision to 07-trace.json for %s: %w", req.TaskID, err)
	}
	return nil
}

// toGenericJSON marshals v and unmarshals it back into an untyped any, yielding
// the map[string]any/[]any/scalar shape the schema validator and append-only
// trace comparison operate on.
func toGenericJSON(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func init() {
	fleetCmd.AddCommand(fleetRouteCmd)
}
