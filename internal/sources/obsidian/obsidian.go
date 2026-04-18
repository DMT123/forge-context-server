// Package obsidian is a Source that reads context from an Obsidian vault.
//
// An Obsidian vault is a directory of markdown files with:
//   - YAML frontmatter (between --- markers)
//   - [[wiki links]] to other notes
//   - #tags inline or in frontmatter
//
// This source respects the vault structure and treats notes as Documents.
// Daily logs (YYYY-MM-DD.md) are treated specially as temporal context.
package obsidian

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DMT123/davzy-vault/internal/sources"
	"github.com/DMT123/davzy-vault/pkg/types"
	"gopkg.in/yaml.v3"
)

// Source reads from an Obsidian vault directory.
type Source struct {
	vaultPath string
	name      string
}

// New creates an Obsidian source rooted at the given vault path.
func New(name, vaultPath string) (*Source, error) {
	info, err := os.Stat(vaultPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("obsidian vault is not a directory: " + vaultPath)
	}
	if name == "" {
		name = "obsidian-" + filepath.Base(vaultPath)
	}
	return &Source{vaultPath: vaultPath, name: name}, nil
}

// Name implements Source.
func (s *Source) Name() string { return s.name }

// Identity is not supported by Obsidian sources (use workspace source for that).
func (s *Source) Identity(ctx context.Context) (*types.Identity, error) {
	return nil, &sources.ErrNotImplemented{SourceName: s.name, Operation: "Identity"}
}

// ListProjects looks for notes with a #project tag or in a "Projects/" folder.
func (s *Source) ListProjects(ctx context.Context, status string) ([]types.Project, error) {
	var projects []types.Project

	// Look in a Projects/ folder if it exists
	projectsDir := filepath.Join(s.vaultPath, "Projects")
	if _, err := os.Stat(projectsDir); err == nil {
		entries, _ := os.ReadDir(projectsDir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				path := filepath.Join(projectsDir, e.Name())
				if p := s.projectFromFile(path); p != nil {
					if status == "" || p.Status == status {
						projects = append(projects, *p)
					}
				}
			}
		}
	}

	// Also scan vault root for anything tagged #project
	_ = filepath.WalkDir(s.vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(path, "Projects/") {
			return nil // already covered
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		body, _ := os.ReadFile(path)
		if !hasTag(string(body), "project") {
			return nil
		}
		if p := s.projectFromFile(path); p != nil {
			if status == "" || p.Status == status {
				projects = append(projects, *p)
			}
		}
		return nil
	})

	sort.Slice(projects, func(i, j int) bool { return projects[i].LastUpdate.After(projects[j].LastUpdate) })
	return projects, nil
}

// SearchDocuments does a substring search across the vault.
// Phase 2: integrate with Obsidian's .obsidian/cache for proper indexing.
func (s *Source) SearchDocuments(ctx context.Context, query string, limit int) ([]types.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, errors.New("empty query")
	}

	var results []types.SearchResult
	_ = filepath.WalkDir(s.vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Skip .obsidian internal files
		if strings.Contains(path, "/.obsidian/") || strings.Contains(path, "/.trash/") {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
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
		_, frontmatter, cleanBody := parseFrontmatter(text)
		tags := extractTags(cleanBody, frontmatter)

		results = append(results, types.SearchResult{
			Document: types.Document{
				ID:        relVault(s.vaultPath, path),
				Title:     titleFrom(text, d.Name()),
				Path:      path,
				Source:    s.name,
				Tags:      tags,
				UpdatedAt: info.ModTime(),
			},
			Score:   1.0,
			Snippet: snippet(cleanBody, query),
		})
		return nil
	})

	sort.Slice(results, func(i, j int) bool {
		return results[i].Document.UpdatedAt.After(results[j].Document.UpdatedAt)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// GetDocument loads a single note.
func (s *Source) GetDocument(ctx context.Context, id string) (*types.Document, error) {
	// id may be with or without .md suffix
	candidates := []string{id, id + ".md"}
	for _, c := range candidates {
		path := filepath.Join(s.vaultPath, c)
		cleaned := filepath.Clean(path)
		if !strings.HasPrefix(cleaned, s.vaultPath) {
			return nil, errors.New("path traversal blocked")
		}
		body, err := os.ReadFile(cleaned)
		if err != nil {
			continue
		}
		info, _ := os.Stat(cleaned)
		_, fm, clean := parseFrontmatter(string(body))
		tags := extractTags(clean, fm)
		return &types.Document{
			ID:        relVault(s.vaultPath, cleaned),
			Title:     titleFrom(string(body), filepath.Base(cleaned)),
			Path:      cleaned,
			Source:    s.name,
			Body:      clean,
			Tags:      tags,
			UpdatedAt: info.ModTime(),
		}, nil
	}
	return nil, errors.New("document not found: " + id)
}

// RecentDocuments returns the N most recently modified vault notes.
func (s *Source) RecentDocuments(ctx context.Context, limit int) ([]types.Document, error) {
	if limit <= 0 {
		limit = 10
	}
	type item struct {
		path string
		info fs.FileInfo
	}
	var all []item
	_ = filepath.WalkDir(s.vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(path, "/.obsidian/") || strings.Contains(path, "/.trash/") {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
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
			ID:        relVault(s.vaultPath, it.path),
			Title:     filepath.Base(it.path),
			Path:      it.path,
			Source:    s.name,
			UpdatedAt: it.info.ModTime(),
		})
	}
	return out, nil
}

// RecentDecisions mines daily logs for decision-style entries.
// Looks for lines starting with "**Decision:**" or "DECISION:" in daily log files.
func (s *Source) RecentDecisions(ctx context.Context, limit int) ([]types.Decision, error) {
	if limit <= 0 {
		limit = 10
	}
	dailyDir := filepath.Join(s.vaultPath, "Daily-Logs")
	if _, err := os.Stat(dailyDir); err != nil {
		return nil, &sources.ErrNotImplemented{SourceName: s.name, Operation: "RecentDecisions"}
	}

	var decisions []types.Decision
	dateRE := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.md$`)
	decisionRE := regexp.MustCompile(`(?im)^\*?\*?decision:?\*?\*?\s*(.+)$`)

	entries, _ := os.ReadDir(dailyDir)
	for _, e := range entries {
		m := dateRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		date, err := time.Parse("2006-01-02", m[1])
		if err != nil {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dailyDir, e.Name()))
		if err != nil {
			continue
		}
		for _, match := range decisionRE.FindAllStringSubmatch(string(body), -1) {
			decisions = append(decisions, types.Decision{
				ID:       e.Name() + ":" + match[1][:minInt(30, len(match[1]))],
				Title:    strings.TrimSpace(match[1]),
				Decision: strings.TrimSpace(match[1]),
				Date:     date,
				Source:   s.name,
			})
		}
	}
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].Date.After(decisions[j].Date) })
	if len(decisions) > limit {
		decisions = decisions[:limit]
	}
	return decisions, nil
}

// Close is a no-op.
func (s *Source) Close() error { return nil }

// --- helpers -----------------------------------------------------------------

func (s *Source) projectFromFile(path string) *types.Project {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	info, _ := os.Stat(path)
	_, fm, clean := parseFrontmatter(string(body))

	p := &types.Project{
		ID:         strings.TrimSuffix(filepath.Base(path), ".md"),
		Name:       titleFrom(string(body), filepath.Base(path)),
		Summary:    firstParagraph(clean),
		SourcePath: path,
		LastUpdate: info.ModTime(),
	}
	if status, ok := fm["status"].(string); ok {
		p.Status = status
	} else {
		p.Status = "active"
	}
	return p
}

func parseFrontmatter(body string) (raw string, data map[string]any, clean string) {
	data = map[string]any{}
	if !strings.HasPrefix(body, "---\n") && !strings.HasPrefix(body, "---\r\n") {
		return "", data, body
	}
	rest := strings.TrimPrefix(body, "---\n")
	rest = strings.TrimPrefix(rest, "---\r\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", data, body
	}
	raw = rest[:idx]
	clean = rest[idx+4:]
	clean = strings.TrimPrefix(clean, "\n")
	_ = yaml.Unmarshal([]byte(raw), &data)
	return
}

func extractTags(body string, frontmatter map[string]any) []string {
	seen := map[string]bool{}
	var tags []string

	if fmTags, ok := frontmatter["tags"].([]any); ok {
		for _, t := range fmTags {
			if s, ok := t.(string); ok && !seen[s] {
				seen[s] = true
				tags = append(tags, s)
			}
		}
	}

	tagRE := regexp.MustCompile(`#([\w-/]+)`)
	for _, m := range tagRE.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			tags = append(tags, m[1])
		}
	}
	return tags
}

func hasTag(body, tag string) bool {
	for _, t := range extractTags(body, nil) {
		if t == tag {
			return true
		}
	}
	return false
}

func titleFrom(body, fallback string) string {
	_, _, clean := parseFrontmatter(body)
	for _, line := range strings.Split(clean, "\n") {
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

func relVault(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
