// Package harness discovers and safely reads the harness documents bundled
// with a TimeKeeper checkout or installed application.
package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxDocumentBytes = 1 << 20

var errUnsafePath = errors.New("unsafe harness path")

// Item describes one discoverable harness document. ID is the stable short
// name accepted by both the CLI and the friendly-URL reader.
type Item struct {
	ID          string
	Path        string
	Description string
}

type manifest struct {
	Skills []string `yaml:"skills"`
}

// FindRoot locates a checkout or installed app that contains the harness
// contract and manifest. TIMEKEEPER_REPO is an explicit override; otherwise
// the current directory and executable directory are searched upward.
func FindRoot() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("TIMEKEEPER_REPO")); explicit != "" {
		if root, ok := rootAt(explicit); ok {
			return root, nil
		}
		return "", fmt.Errorf("TIMEKEEPER_REPO=%q does not contain harness/manifest.yaml and harness/SKILL.md", explicit)
	}
	candidates := make([]string, 0, 2)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(executable))
	}
	for _, candidate := range candidates {
		if root, ok := rootAt(candidate); ok {
			return root, nil
		}
	}
	return "", errors.New("bundled harness not found; run from a TimeKeeper checkout or installed app containing harness/manifest.yaml")
}

func rootAt(start string) (string, bool) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	dir := filepath.Clean(absolute)
	for depth := 0; depth < 16; depth++ {
		if regularFile(filepath.Join(dir, "harness", "manifest.yaml")) && regularFile(filepath.Join(dir, "harness", "SKILL.md")) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Entries returns the core contract, manifest, available first-use guides, and
// every Skill registered in harness/manifest.yaml.
func Entries(root string) ([]Item, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve harness root: %w", err)
	}
	manifestPath := filepath.Join(root, "harness", "manifest.yaml")
	manifestBytes, err := readFile(root, "harness/manifest.yaml")
	if err != nil {
		return nil, fmt.Errorf("read harness manifest: %w", err)
	}
	var inventory manifest
	if err := yaml.Unmarshal(manifestBytes, &inventory); err != nil {
		return nil, fmt.Errorf("parse harness manifest %q: %w", manifestPath, err)
	}
	if _, err := readFile(root, "harness/SKILL.md"); err != nil {
		return nil, fmt.Errorf("read harness contract: %w", err)
	}

	items := []Item{
		{ID: "core", Path: "harness/SKILL.md", Description: "Operational contract for TimeKeeper agents"},
		{ID: "manifest", Path: "harness/manifest.yaml", Description: "Machine-readable harness and role inventory"},
	}
	for _, item := range []Item{
		{ID: "first-use", Path: "docs/HARNESS.md", Description: "Agent first-use guide"},
		{ID: "agent-router", Path: "docs/agent.md", Description: "Role and harness selection guide"},
	} {
		if _, err := readFile(root, item.Path); err == nil {
			items = append(items, item)
		}
	}
	// Validate every manifest Skill path for compatibility and to fail closed on
	// malformed or escaping entries, while discovery below also picks up new
	// supported files that have not yet been added to the manifest.
	for _, rawPath := range inventory.Skills {
		path, err := cleanItemPath(rawPath)
		if err != nil || path != rawPath || !strings.HasPrefix(path, "harness/skills/") || !strings.HasSuffix(path, "/SKILL.md") || strings.Count(path, "/") != 3 {
			return nil, fmt.Errorf("manifest contains invalid harness Skill path %q", rawPath)
		}
		if _, err := readFile(root, path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("manifest Skill %q is unavailable: %w", path, err)
		}
	}
	// Discover supported documents from the harness tree itself. The manifest is
	// still parsed/validated above as the machine-readable contract, but the web
	// menu and CLI inventory do not need hand-maintained entries for each file.
	harnessDir := filepath.Join(root, "harness")
	err = filepath.WalkDir(harnessDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "harness/SKILL.md" || rel == "harness/manifest.yaml" {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".md" && ext != ".yaml" && ext != ".yml" {
			return nil
		}
		content, err := readFile(root, rel)
		if err != nil {
			return fmt.Errorf("read harness document %q: %w", rel, err)
		}
		stem := strings.TrimSuffix(rel, filepath.Ext(rel))
		id := strings.TrimPrefix(stem, "harness/")
		if strings.HasPrefix(rel, "harness/skills/") && strings.HasSuffix(rel, "/SKILL.md") {
			id = filepath.Base(filepath.Dir(path))
		} else {
			id = strings.ReplaceAll(id, "/", "-")
		}
		id = strings.ToLower(id)
		if !validID(id) {
			return fmt.Errorf("harness document %q produces invalid identifier %q", rel, id)
		}
		description := skillDescription(content)
		if description == "Harness Skill" {
			description = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		}
		items = append(items, Item{ID: id, Path: rel, Description: description})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover harness documents: %w", err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for i := 1; i < len(items); i++ {
		if items[i-1].ID == items[i].ID {
			return nil, fmt.Errorf("duplicate harness item identifier %q", items[i].ID)
		}
	}
	return items, nil
}

// Lookup reads a listed item by its stable ID or exact relative path. Arbitrary
// filesystem paths are never accepted.
func Lookup(root, identifier string) (Item, []byte, error) {
	items, err := Entries(root)
	if err != nil {
		return Item{}, nil, err
	}
	identifier = strings.TrimSpace(identifier)
	for _, item := range items {
		if identifier != item.ID && identifier != item.Path {
			continue
		}
		content, err := readFile(root, item.Path)
		if err != nil {
			return Item{}, nil, fmt.Errorf("read harness item %s: %w", item.ID, err)
		}
		return item, content, nil
	}
	return Item{}, nil, fmt.Errorf("harness item %q is not listed", identifier)
}

// QueryIdentifier recognizes either the compact link form ?skill-name.md or
// the explicit form ?harness=skill-name. Unrelated dashboard query strings are
// left to the dashboard unchanged.
func QueryIdentifier(rawQuery string) (string, bool) {
	if rawQuery == "" {
		return "", false
	}
	if !strings.Contains(rawQuery, "=") {
		decoded, err := url.QueryUnescape(rawQuery)
		if err != nil {
			return "", false
		}
		if id, ok := stripSupportedExtension(decoded); ok {
			return id, true
		}
		return "", false
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", false
	}
	ids := values["harness"]
	if len(ids) != 1 || strings.TrimSpace(ids[0]) == "" {
		return "", false
	}
	id := strings.TrimSpace(ids[0])
	if stripped, ok := stripSupportedExtension(id); ok {
		id = stripped
	}
	return id, true
}

func stripSupportedExtension(value string) (string, bool) {
	for _, extension := range []string{".md", ".yaml", ".yml", ".html"} {
		if len(value) > len(extension) && strings.EqualFold(value[len(value)-len(extension):], extension) {
			return value[:len(value)-len(extension)], true
		}
	}
	return "", false
}

func validID(id string) bool {
	if id == "" || id[0] < 'a' || id[0] > 'z' {
		return false
	}
	for _, char := range id[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func cleanItemPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || strings.Contains(raw, "\\") || filepath.IsAbs(raw) {
		return "", errUnsafePath
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errUnsafePath
	}
	return clean, nil
}

func readFile(root, relative string) ([]byte, error) {
	clean, err := cleanItemPath(relative)
	if err != nil {
		return nil, err
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve harness root: %w", err)
	}
	candidate := filepath.Join(rootReal, filepath.FromSlash(clean))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(rootReal, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return nil, errUnsafePath
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("harness item %q is not a regular file", clean)
	}
	if info.Size() > maxDocumentBytes {
		return nil, fmt.Errorf("harness item %q exceeds the %d-byte read limit", clean, maxDocumentBytes)
	}
	return os.ReadFile(resolved)
}

func skillDescription(content []byte) string {
	parts := strings.SplitN(string(content), "---", 3)
	if len(parts) < 3 {
		return "Harness Skill"
	}
	for _, line := range strings.Split(parts[1], "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "description:"); ok {
			return strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	return "Harness Skill"
}
