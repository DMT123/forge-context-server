// Package types defines the core domain types for the Forge Context Server.
package types

import "time"

// Identity describes who the context owner is (persona, style, values).
type Identity struct {
	Name         string            `json:"name"`
	Role         string            `json:"role"`
	Organisation string            `json:"organisation"`
	Location     string            `json:"location"`
	Timezone     string            `json:"timezone"`
	Bio          string            `json:"bio"`
	Values       []string          `json:"values"`
	Preferences  map[string]string `json:"preferences"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// Project captures a live or archived workstream.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"` // active | parked | archived | reference
	Summary     string    `json:"summary"`
	Stack       []string  `json:"stack,omitempty"`
	Links       []Link    `json:"links,omitempty"`
	Owners      []string  `json:"owners,omitempty"`
	LastUpdate  time.Time `json:"last_update"`
	SourcePath  string    `json:"source_path,omitempty"`
}

// Document is any markdown/text file with metadata.
type Document struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Path        string    `json:"path"`
	Source      string    `json:"source"` // obsidian | workspace | github
	Summary     string    `json:"summary,omitempty"`
	Body        string    `json:"body,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Decision captures a recorded choice (promotes from Obsidian/workspace notes).
type Decision struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Context   string    `json:"context"`
	Decision  string    `json:"decision"`
	Rationale string    `json:"rationale"`
	Date      time.Time `json:"date"`
	Source    string    `json:"source"`
}

// Link is a typed reference to an external resource.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Kind  string `json:"kind,omitempty"` // github | docs | live | other
}

// SearchResult is returned from any source's search method.
type SearchResult struct {
	Document Document `json:"document"`
	Score    float64  `json:"score"`
	Snippet  string   `json:"snippet,omitempty"`
}

// ContextBundle is a composite view for quickly briefing a new agent.
type ContextBundle struct {
	Identity        *Identity   `json:"identity"`
	ActiveProjects  []Project   `json:"active_projects"`
	RecentDocuments []Document  `json:"recent_documents"`
	RecentDecisions []Decision  `json:"recent_decisions"`
	GeneratedAt     time.Time   `json:"generated_at"`
}
