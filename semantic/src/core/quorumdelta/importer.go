package quorumdelta

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/hsme/core/src/core/indexer"
)

type ImportResult struct {
	Fetched  int
	Skipped  int
	Ingested int
	Errored  int
}

func renderRawContent(entry QuorumMemoryEntry) string {
	var sb strings.Builder
	sb.WriteString("Type: ")
	sb.WriteString(entry.Type)
	sb.WriteString("\nTitle: ")
	sb.WriteString(entry.Title)
	sb.WriteString("\nContext: ")
	sb.WriteString(entry.Context)
	sb.WriteString("\nContent:\n")
	sb.WriteString(entry.Content)
	sb.WriteString("\nSource Task: ")
	sb.WriteString(entry.SourceTask)
	sb.WriteString("\nCreated At: ")
	sb.WriteString(entry.CreatedAt)
	return sb.String()
}

func Import(quorumDB *sql.DB, hsmeDB *sql.DB, quorumProjectID string, hsmeProject string) (*ImportResult, error) {
	if strings.TrimSpace(hsmeProject) == "" {
		return nil, fmt.Errorf("hsmeProject cannot be empty")
	}
	if strings.TrimSpace(quorumProjectID) == "" {
		return nil, fmt.Errorf("quorumProjectID cannot be empty")
	}

	entries, err := FetchEntries(quorumDB, quorumProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch quorum entries: %w", err)
	}

	result := &ImportResult{
		Fetched: len(entries),
	}

	for _, entry := range entries {
		// Skip if already in mapping table
		existingHSMEID, err := LookupHSMEID(hsmeDB, hsmeProject, entry.ID)
		if err != nil {
			result.Errored++
			return result, fmt.Errorf("failed to check existing mapping for entry %s: %w", entry.ID, err)
		}
		if existingHSMEID != nil {
			result.Skipped++
			continue
		}

		// Resolve supersedes edge
		var supersedesHSMEID *int64
		if entry.Supersedes != nil && *entry.Supersedes != "" {
			resolvedID, err := LookupHSMEID(hsmeDB, hsmeProject, *entry.Supersedes)
			if err != nil {
				// Non-fatal per ADR 0013 / spec tolerance: proceed without supersedes edge if lookup error
				supersedesHSMEID = nil
			} else {
				supersedesHSMEID = resolvedID
			}
		}

		rawContent := renderRawContent(entry)
		hsmeMemID, err := indexer.StoreContext(hsmeDB, rawContent, "quorum_curated_memory", hsmeProject, supersedesHSMEID, false)
		if err != nil {
			result.Errored++
			return result, fmt.Errorf("failed to store context for entry %s: %w", entry.ID, err)
		}

		if err := RecordMapping(hsmeDB, hsmeProject, entry.ID, hsmeMemID); err != nil {
			result.Errored++
			return result, fmt.Errorf("failed to record mapping for entry %s: %w", entry.ID, err)
		}

		result.Ingested++
	}

	return result, nil
}
