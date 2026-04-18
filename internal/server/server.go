// Package server wires MCP tools to source backends and runs the transport.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/DMT123/davzy-vault/internal/config"
	"github.com/DMT123/davzy-vault/internal/sources"
	"github.com/DMT123/davzy-vault/pkg/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Forge is the top-level context server.
type Forge struct {
	cfg     *config.Config
	sources []sources.Source
	logger  *slog.Logger
}

// New constructs a Forge with the given config and pre-built sources.
func New(cfg *config.Config, srcs []sources.Source, logger *slog.Logger) *Forge {
	return &Forge{cfg: cfg, sources: srcs, logger: logger}
}

// Run starts the server on the configured transport.
func (f *Forge) Run(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    f.cfg.Server.Name,
		Version: f.cfg.Server.Version,
	}, nil)

	f.registerTools(server)

	switch strings.ToLower(f.cfg.Server.Transport) {
	case "stdio":
		f.logger.Info("starting stdio transport")
		return server.Run(ctx, &mcp.StdioTransport{})
	case "http", "sse":
		addr := fmt.Sprintf("%s:%d", f.cfg.Server.Host, f.cfg.Server.Port)
		f.logger.Info("starting http transport", slog.String("addr", addr))
		// DisableLocalhostProtection: required because we front this via Cloudflare
		// Tunnel / Tailscale where the public Host header differs from the bind address.
		// We still rely on Tailscale ACLs / Cloudflare Access for real authentication.
		opts := &mcp.StreamableHTTPOptions{
			DisableLocalhostProtection: true,
		}
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, opts)
		return http.ListenAndServe(addr, handler)
	default:
		return errors.New("unsupported transport: " + f.cfg.Server.Transport)
	}
}

// --- tool registration -------------------------------------------------------

type identityInput struct{}
type identityOutput struct {
	Identity *types.Identity `json:"identity"`
	Source   string          `json:"source"`
}

type projectsInput struct {
	Status string `json:"status,omitempty" jsonschema:"optional filter: active | parked | archived | reference"`
}
type projectsOutput struct {
	Projects []types.Project `json:"projects"`
	Count    int             `json:"count"`
}

type searchInput struct {
	Query string `json:"query" jsonschema:"search query string (required)"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 20)"`
}
type searchOutput struct {
	Results []types.SearchResult `json:"results"`
	Count   int                  `json:"count"`
}

type getDocInput struct {
	ID     string `json:"id" jsonschema:"document id / relative path"`
	Source string `json:"source,omitempty" jsonschema:"optional source name; omit to search all"`
}
type getDocOutput struct {
	Document *types.Document `json:"document"`
}

type bundleInput struct {
	ProjectLimit  int `json:"project_limit,omitempty"`
	DocumentLimit int `json:"document_limit,omitempty"`
	DecisionLimit int `json:"decision_limit,omitempty"`
}
type bundleOutput struct {
	Bundle *types.ContextBundle `json:"bundle"`
}

type addMemoryInput struct {
	Title     string   `json:"title" jsonschema:"short title for the memory (required)"`
	Body      string   `json:"body" jsonschema:"full markdown body of the memory (required)"`
	Source    string   `json:"source,omitempty" jsonschema:"originating agent: claude | chatgpt | hector | other (default: other)"`
	Tags      []string `json:"tags,omitempty" jsonschema:"optional list of tags"`
}
type addMemoryOutput struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

func (f *Forge) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_identity",
		Description: "Return the context owner's identity (name, role, bio, values).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ identityInput) (*mcp.CallToolResult, identityOutput, error) {
		for _, src := range f.sources {
			id, err := src.Identity(ctx)
			if err == nil && id != nil {
				return nil, identityOutput{Identity: id, Source: src.Name()}, nil
			}
		}
		return nil, identityOutput{}, errors.New("no source returned an identity")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List projects across all sources, optionally filtered by status.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in projectsInput) (*mcp.CallToolResult, projectsOutput, error) {
		var all []types.Project
		for _, src := range f.sources {
			ps, err := src.ListProjects(ctx, in.Status)
			if err != nil {
				f.logger.Warn("list_projects: source failed", slog.String("source", src.Name()), slog.Any("err", err))
				continue
			}
			all = append(all, ps...)
		}
		return nil, projectsOutput{Projects: all, Count: len(all)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_documents",
		Description: "Search across all sources for documents matching the query.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		if strings.TrimSpace(in.Query) == "" {
			return nil, searchOutput{}, errors.New("query is required")
		}
		var all []types.SearchResult
		for _, src := range f.sources {
			rs, err := src.SearchDocuments(ctx, in.Query, in.Limit)
			if err != nil {
				f.logger.Warn("search_documents: source failed", slog.String("source", src.Name()), slog.Any("err", err))
				continue
			}
			all = append(all, rs...)
		}
		return nil, searchOutput{Results: all, Count: len(all)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document",
		Description: "Fetch a single document's full body by id. Optionally filter to one source.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getDocInput) (*mcp.CallToolResult, getDocOutput, error) {
		for _, src := range f.sources {
			if in.Source != "" && in.Source != src.Name() {
				continue
			}
			doc, err := src.GetDocument(ctx, in.ID)
			if err == nil && doc != nil {
				return nil, getDocOutput{Document: doc}, nil
			}
		}
		return nil, getDocOutput{}, errors.New("document not found in any source")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_memory",
		Description: "Persist a new memory/observation to the context server. Any agent can write. Returns the stored memory's id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in addMemoryInput) (*mcp.CallToolResult, addMemoryOutput, error) {
		if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Body) == "" {
			return nil, addMemoryOutput{}, errors.New("title and body are required")
		}
		for _, src := range f.sources {
			writer, ok := src.(sources.WriteableSource)
			if !ok {
				continue
			}
			id, err := writer.AddMemory(ctx, in.Title, in.Body, in.Source, in.Tags)
			if err != nil {
				f.logger.Warn("add_memory: source failed", slog.String("source", src.Name()), slog.Any("err", err))
				continue
			}
			f.logger.Info("memory added", slog.String("source", src.Name()), slog.String("id", id))
			return nil, addMemoryOutput{ID: id, Source: src.Name()}, nil
		}
		return nil, addMemoryOutput{}, errors.New("no writeable source available")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_context_bundle",
		Description: "Compact bundle for briefing a new agent: identity + active projects + recent docs + recent decisions.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in bundleInput) (*mcp.CallToolResult, bundleOutput, error) {
		b := &types.ContextBundle{GeneratedAt: time.Now().UTC()}
		if in.ProjectLimit == 0 {
			in.ProjectLimit = 10
		}
		if in.DocumentLimit == 0 {
			in.DocumentLimit = 5
		}
		if in.DecisionLimit == 0 {
			in.DecisionLimit = 5
		}

		for _, src := range f.sources {
			if b.Identity == nil {
				if id, err := src.Identity(ctx); err == nil {
					b.Identity = id
				}
			}
			if ps, err := src.ListProjects(ctx, "active"); err == nil {
				b.ActiveProjects = append(b.ActiveProjects, ps...)
			}
			if ds, err := src.RecentDocuments(ctx, in.DocumentLimit); err == nil {
				b.RecentDocuments = append(b.RecentDocuments, ds...)
			}
			if decs, err := src.RecentDecisions(ctx, in.DecisionLimit); err == nil {
				b.RecentDecisions = append(b.RecentDecisions, decs...)
			}
		}

		if len(b.ActiveProjects) > in.ProjectLimit {
			b.ActiveProjects = b.ActiveProjects[:in.ProjectLimit]
		}
		if len(b.RecentDocuments) > in.DocumentLimit {
			b.RecentDocuments = b.RecentDocuments[:in.DocumentLimit]
		}
		if len(b.RecentDecisions) > in.DecisionLimit {
			b.RecentDecisions = b.RecentDecisions[:in.DecisionLimit]
		}
		return nil, bundleOutput{Bundle: b}, nil
	})
}
