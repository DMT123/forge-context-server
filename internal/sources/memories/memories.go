// Package memories is a Source that reads captured AI agent memories.
//
// Expected structure under root:
//
//	root/
//	  claude-desktop/
//	    memories/*.md        # manual exports of Claude memory
//	    conversations/*.json # exported conversations (optional)
//	  chatgpt/
//	    memories.md          # pasted from Settings → Memory
//	    conversations/*.json # from official data export
//	  other/
//	    *.md                 # anything else worth searching
//
// This is a first-cut source — a future version will also WRITE memories
// (via an MCP tool `add_memory`) so agents can persist observations.
package memories

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DMT123/davzy-vault/internal/sources"
	"github.com/DMT123/davzy-vault/pkg/types"
)

// Source reads memory dumps from a directory tree.
type Source struct {
	root string
	name string
}

// New returns a memories source. Creates the root dir if missing.
func New(name, root string) (*Source, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	// Seed the layout the first time
	for _, sub := range []string{"claude-desktop", "chatgpt", "other"} {
		_ = os.MkdirAll(filepath.Join(root, sub), 0o755)
	}
	if name == "" {
		name = "memories"
	}
	return &Source{root: root, name: name}, nil
}

// Name implements Source.
func (s *Source) Name() string { return s.name }

// Identity: not supported — memories are about what agents remember, not the
// persona definition itself.
func (s *Source) Identity(ctx context.Context) (*types.Identity, error) {
	return nil, &sources.ErrNotImplemented{SourceName: s.name, Operation: "Identity"}
}

// ListProjects: not supported yet. Memories could include project context,
// but we don't try to infer projects from them here.
func (s *Source) ListProjects(ctx context.Context, status string) ([]types.Project, error) {
	return nil, &sources.ErrNotImplemented{SourceName: s.name, Operation: "ListProjects"}
}

// SearchDocuments does a substring search across all memory files.
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
		if err != nil || d.IsDir() {
			return nil
		}
		if !isTextFile(d.Name()) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := string(body)
		if !strings.Contains(strings.ToLower(text), query) {
			return nil
		}
		info, _ := d.Info()
		results = append(results, types.SearchResult{
			Document: types.Document{
				ID:        relPath(s.root, path),
				Title:     filepath.Base(path),
				Path:      path,
				Source:    s.name,
				Tags:      []string{inferMemoryTag(path)},
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

// GetDocument loads a single memory file.
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
		Title:     filepath.Base(path),
		Path:      path,
		Source:    s.name,
		Body:      string(body),
		Tags:      []string{inferMemoryTag(path)},
		UpdatedAt: info.ModTime(),
	}, nil
}

// RecentDocuments returns the N most recently modified memory files.
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
		if err != nil || d.IsDir() || !isTextFile(d.Name()) {
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
			Source:    s.name,
			Tags:      []string{inferMemoryTag(it.path)},
			UpdatedAt: it.info.ModTime(),
		})
	}
	return out, nil
}

// RecentDecisions: not supported yet for memories.
func (s *Source) RecentDecisions(ctx context.Context, limit int) ([]types.Decision, error) {
	return nil, &sources.ErrNotImplemented{SourceName: s.name, Operation: "RecentDecisions"}
}

// Close is a no-op.
func (s *Source) Close() error { return nil }

// AddMemory persists a new memory as a markdown file. sourceTag controls the
// subdirectory ("claude" → claude-desktop, "chatgpt" → chatgpt, anything else
// → other/<sourceTag>). Returns the relative id for later GetDocument.
func (s *Source) AddMemory(ctx context.Context, title, body, sourceTag string, tags []string) (string, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" || body == "" {
		return "", errors.New("title and body are required")
	}

	// Route by source tag
	var subdir string
	switch strings.ToLower(sourceTag) {
	case "claude", "claude-desktop", "anthropic":
		subdir = "claude-desktop"
	case "chatgpt", "openai":
		subdir = "chatgpt"
	case "", "other":
		subdir = "other"
	default:
		subdir = filepath.Join("other", slugify(sourceTag))
	}

	dir := filepath.Join(s.root, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	// Timestamped filename
	now := time.Now().UTC()
	base := fmt.Sprintf("%s-%s.md", now.Format("2006-01-02-150405"), slugify(title))
	fullPath := filepath.Join(dir, base)

	// Assemble markdown with YAML frontmatter
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "title: %s\n", escapeYAML(title))
	fmt.Fprintf(&sb, "source: %s\n", sourceTag)
	fmt.Fprintf(&sb, "captured_at: %s\n", now.Format(time.RFC3339))
	if len(tags) > 0 {
		sb.WriteString("tags:\n")
		for _, t := range tags {
			fmt.Fprintf(&sb, "  - %s\n", t)
		}
	}
	sb.WriteString("---\n\n")
	sb.WriteString("# " + title + "\n\n")
	sb.WriteString(body)
	sb.WriteString("\n")

	if err := os.WriteFile(fullPath, []byte(sb.String()), 0o644); err != nil {
		return "", err
	}

	return relPath(s.root, fullPath), nil
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "memory"
	}
	return s
}

func escapeYAML(s string) string {
	if strings.ContainsAny(s, ":\"'#\n") {
		return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	}
	return s
}

// --- helpers ---

func isTextFile(name string) bool {
	for _, ext := range []string{".md", ".txt", ".json"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func inferMemoryTag(path string) string {
	if strings.Contains(path, "/claude-desktop/") {
		return "claude"
	}
	if strings.Contains(path, "/chatgpt/") {
		return "chatgpt"
	}
	return "other"
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
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
