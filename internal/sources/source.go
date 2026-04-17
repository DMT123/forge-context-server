// Package sources defines the Source interface and plugin registry.
//
// A Source is any backend that can provide context — filesystem, Obsidian,
// GitHub, Postgres, Notion, etc. Each source implements a subset of the
// interface methods; the server aggregates them.
package sources

import (
	"context"

	"github.com/DMT123/forge-context-server/pkg/types"
)

// Source is the contract every context backend must implement.
// Methods return ErrNotImplemented if the source does not support that op.
type Source interface {
	// Name returns a stable identifier (e.g. "workspace", "obsidian").
	Name() string

	// Identity returns the owner's identity, if the source has one.
	Identity(ctx context.Context) (*types.Identity, error)

	// ListProjects returns known projects. status filters: "active", "parked",
	// "archived", "reference", or "" for all.
	ListProjects(ctx context.Context, status string) ([]types.Project, error)

	// SearchDocuments returns documents matching query. limit <=0 = server default.
	SearchDocuments(ctx context.Context, query string, limit int) ([]types.SearchResult, error)

	// GetDocument returns a single document by id/path.
	GetDocument(ctx context.Context, id string) (*types.Document, error)

	// RecentDocuments returns the N most recently modified documents.
	RecentDocuments(ctx context.Context, limit int) ([]types.Document, error)

	// RecentDecisions returns the N most recent decisions (if supported).
	RecentDecisions(ctx context.Context, limit int) ([]types.Decision, error)

	// Close releases any resources.
	Close() error
}

// WriteableSource is implemented by sources that can persist new items.
type WriteableSource interface {
	Source

	// AddMemory persists a new memory/observation. sourceTag is e.g.
	// "claude", "chatgpt", "hector" — used to file under the right subdir.
	// Returns the relative id of the stored item.
	AddMemory(ctx context.Context, title, body, sourceTag string, tags []string) (string, error)
}

// ErrNotImplemented indicates a source does not support this operation.
type ErrNotImplemented struct {
	SourceName string
	Operation  string
}

func (e *ErrNotImplemented) Error() string {
	return "source " + e.SourceName + " does not implement " + e.Operation
}
