package main

import (
	"database/sql"
	"fmt"
	"regexp"
)

var wrapperRegex = regexp.MustCompile(`(?s)^Title: (.*?)\nProject: (.*?)\nType: (.*?)\n\n(.*)$`)

type WrappedMemory struct {
	Title   string
	Project string
	Type    string
	Content string
}

func ParseWrapper(raw string) (*WrappedMemory, error) {
	matches := wrapperRegex.FindStringSubmatch(raw)
	if matches == nil {
		return nil, fmt.Errorf("unparseable wrapper format")
	}

	return &WrappedMemory{
		Title:   matches[1],
		Project: matches[2],
		Type:    matches[3],
		Content: matches[4],
	}, nil
}

type LegacyObservation struct {
	ID        int
	Type      string
	Title     string
	Content   string
	Project   string
	CreatedAt string
}

func LoadLegacyObservations(db *sql.DB) (map[string]*LegacyObservation, error) {
	rows, err := db.Query("SELECT id, type, title, content, project, created_at FROM observations WHERE deleted_at IS NULL")
	if err != nil {
		return nil, fmt.Errorf("failed to query legacy observations: %w", err)
	}
	defer rows.Close()

	obsMap := make(map[string]*LegacyObservation)
	for rows.Next() {
		o := &LegacyObservation{}
		if err := rows.Scan(&o.ID, &o.Type, &o.Title, &o.Content, &o.Project, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan legacy observation: %w", err)
		}
		// Exact content is the key
		obsMap[o.Content] = o
	}

	return obsMap, nil
}
