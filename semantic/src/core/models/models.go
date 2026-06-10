package models

import "time"

type MemoryDocument struct {
	ID           int64     `json:"id"`
	RawContent   string    `json:"raw_content"`
	ContentHash  string    `json:"content_hash"`
	SourceType   string    `json:"source_type"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	SupersededBy *int64    `json:"superseded_by"`
	Status       string    `json:"status"`
}

type MemoryChunk struct {
	ID            int64  `json:"id"`
	MemoryID      int64  `json:"memory_id"`
	ChunkIndex    int    `json:"chunk_index"`
	ChunkText     string `json:"chunk_text"`
	TokenEstimate int    `json:"token_estimate"`
}

type KGNode struct {
	ID            int64  `json:"id"`
	Type          string `json:"type"`
	CanonicalName string `json:"canonical_name"`
	DisplayName   string `json:"display_name"`
}

type KGEdgeEvidence struct {
	SourceNodeID int64  `json:"source_node_id"`
	TargetNodeID int64  `json:"target_node_id"`
	RelationType string `json:"relation_type"`
	MemoryID     int64  `json:"memory_id"`
}

type AsyncTask struct {
	ID           int64      `json:"id"`
	MemoryID     int64      `json:"memory_id"`
	TaskType     string     `json:"task_type"`
	Status       string     `json:"status"`
	AttemptCount int        `json:"attempt_count"`
	LastError    *string    `json:"last_error"`
	LeasedUntil  *time.Time `json:"leased_until"`
}
