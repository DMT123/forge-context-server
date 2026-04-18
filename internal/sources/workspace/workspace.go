// Package workspace is a Source that reads context from a local filesystem tree.
//
// Expected layout (soft — missing files simply yield empty results):
//
//	<root>/
//	  IDENTITY.md         // YAML frontmatter => Identity
//	  SOUL.md             // appended to Identity.Bio
//	  USER.md             // appended to Identity.Bio
//	  MEMORY.md           // mined for projects + decisions
//	  projects/*.md       // individual project files (optional)
//	  decisions/*.md      // ADR-style decision records (optional)
//	  **/*.md             // every markdown file is a Document
package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DMT123/davzy-vault/internal/sources"
	"github.com/DMT123/davzy-vault/pkg/types"
)

// Source reads context from a workspace directory.
type Source struct {
	root string
}

// New returns a workspace source rooted at path.
func New(root string) (*Source, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("workspace root is not a directory: " + root)
	}
	return &Source{root: root}, nil
}

// Name implements Source.
func (s *Source) Name() string { return "workspace" }

// Identity returns a best-effort Identity from IDENTITY.md / SOUL.md / USER.md.
func (s *Source) Identity(ctx context.Context) (*types.Identity, error) {
	id := &types.Identity{Preferences: map[string]string{}}
	parts := []string{}

	for _, fname := range []string{"IDENTITY.md", "SOUL.md", "USER.md"} {
		body, err := os.ReadFile(filepath.Join(s.root, fname))
		if err != nil {
			continue
		}
		parts = append(parts, string(body))
	}

	if len(parts) == 0 {
		return nil, &sources.ErrNotImplemented{SourceName: s.Name(), Operation: "Identity"}
	}

	id.Bio = strings.Join(parts, "\n\n---\n\n")
	// Very light heuristic extraction — real metadata extraction happens
	// via YAML frontmatter in a follow-up.
	id.Name = firstLineField(parts[0], "Name:", "")
	id.Role = firstLineField(parts[0], "Role:", "")
	id.Location = firstLineField(parts[0], "Location:", "")
	return id, nil
}

// ListProjects scans for projects/*.md and returns summaries.
func (s *Source) ListProjects(ctx context.Context, status string) ([]types.Project, error) {
	dir := filepath.Join(s.root, "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []types.Project{}, nil
		}
		return nil, err
	}

	var out []types.Project
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		info, _ := e.Info()
		p := types.Project{
			ID:         strings.TrimSuffix(e.Name(), ".md"),
			Name:       titleFrom(string(body), e.Name()),
			Summary:    firstParagraph(string(body)),
			Status:     firstLineField(string(body), "Status:", "active"),
			SourcePath: path,
			LastUpdate: info.ModTime(),
		}
		if status == "" || p.Status == status {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUpdate.After(out[j].LastUpdate) })
	return out, nil
}

// SearchDocuments does a naive case-insensitive substring search.
// For Phase 2 we'll add Bleve / SQLite FTS5 for real ranking.
func (s *Source) SearchDocuments(ctx context.Context, query string, limit int) ([]types.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, errors.New("empty query")
	}

	var results []types.SearchResult
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := string(body)
		lower := strings.ToLower(text)
		if !strings.Contains(lower, query) {
			return nil
		}
		info, _ := d.Info()
		results = append(results, types.SearchResult{
			Document: types.Document{
				ID:        relPath(s.root, path),
				Title:     titleFrom(text, d.Name()),
				Path:      path,
				Source:    s.Name(),
				Summary:   snippet(text, query),
				UpdatedAt: info.ModTime(),
			},
			Score:   1.0,
			Snippet: snippet(text, query),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Document.UpdatedAt.After(results[j].Document.UpdatedAt)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// GetDocument loads a single markdown file by relative id.
func (s *Source) GetDocument(ctx context.Context, id string) (*types.Document, error) {
	path := filepath.Join(s.root, id)
	if !strings.HasPrefix(filepath.Clean(path), s.root) {
		return nil, errors.New("path traversal blocked")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, _ := os.Stat(path)
	return &types.Document{
		ID:        id,
		Title:     titleFrom(string(body), filepath.Base(path)),
		Path:      path,
		Source:    s.Name(),
		Body:      string(body),
		UpdatedAt: info.ModTime(),
	}, nil
}

// RecentDocuments returns the N most recently modified markdown files.
func (s *Source) RecentDocuments(ctx context.Context, limit int) ([]types.Document, error) {
	if limit <= 0 {
		limit = 10
	}
	type item struct {
		path string
		info fs.FileInfo
	}
	var all []item
	_ = filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		info, _ := d.Info()
		all = append(all, item{path: path, info: info})
		return nil
	})
	sort.Slice(all, func(i, j int) bool { return all[i].info.ModTime().After(all[j].info.ModTime()) })
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]types.Document, 0, len(all))
	for _, it := range all {
		out = append(out, types.Document{
			ID:        relPath(s.root, it.path),
			Title:     filepath.Base(it.path),
			Path:      it.path,
			Source:    s.Name(),
			UpdatedAt: it.info.ModTime(),
		})
	}
	return out, nil
}

// RecentDecisions is not yet implemented for the workspace source.
func (s *Source) RecentDecisions(ctx context.Context, limit int) ([]types.Decision, error) {
	return nil, &sources.ErrNotImplemented{SourceName: s.Name(), Operation: "RecentDecisions"}
}

// Close is a no-op.
func (s *Source) Close() error { return nil }

// --- helpers -----------------------------------------------------------------

func firstLineField(body, prefix, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- **"+prefix) || strings.HasPrefix(line, "**"+prefix) || strings.HasPrefix(line, prefix) {
			v := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			v = strings.Trim(v, "* _`")
			if v != "" {
				return v
			}
		}
	}
	return fallback
}

func titleFrom(body, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return strings.TrimSuffix(fallback, ".md")
}

func firstParagraph(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 200 {
			return line[:200] + "…"
		}
		return line
	}
	return ""
}

func snippet(body, query string) string {
	lower := strings.ToLower(body)
	idx := strings.Index(lower, query)
	if idx < 0 {
		if len(body) > 200 {
			return body[:200]
		}
		return body
	}
	start := idx - 80
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 80
	if end > len(body) {
		end = len(body)
	}
	return "…" + body[start:end] + "…"
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
