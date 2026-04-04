package mention

import (
	"bufio"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const defaultSearchLimit = 15

var (
	mentionIndexRefreshInterval = 10 * time.Second
	mentionIndexDefaultMaxFiles = 6000
)

type Token struct {
	Query string
	Start int
	End   int
}

type Candidate struct {
	Path     string
	BaseName string
	TypeTag  string
}

type IndexStats struct {
	Count     int
	MaxFiles  int
	Truncated bool
	Ready     bool
}

type mentionIgnoreMatcher struct {
	exact map[string]struct{}
	globs []string
}

type WorkspaceFileIndex struct {
	mu        sync.RWMutex
	root      string
	ready     bool
	building  bool
	lastBuild time.Time
	files     []Candidate
	truncated bool
	maxFiles  int
}

func NewWorkspaceFileIndex(workspace string) *WorkspaceFileIndex {
	return &WorkspaceFileIndex{
		root:     strings.TrimSpace(workspace),
		maxFiles: mentionMaxFilesFromEnv(),
	}
}

func NewStaticWorkspaceFileIndex(candidates []Candidate, maxFiles int, truncated bool) *WorkspaceFileIndex {
	defaultMax := maxFiles <= 0
	copied := make([]Candidate, 0, len(candidates))
	for _, item := range candidates {
		candidate := item
		candidate.Path = filepath.ToSlash(strings.TrimSpace(candidate.Path))
		if candidate.Path == "" {
			continue
		}
		if strings.TrimSpace(candidate.BaseName) == "" {
			candidate.BaseName = filepath.Base(candidate.Path)
		}
		if strings.TrimSpace(candidate.TypeTag) == "" {
			candidate.TypeTag = mentionTypeTag(candidate.Path)
		}
		copied = append(copied, candidate)
	}
	if defaultMax {
		maxFiles = len(copied)
	}
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].Path < copied[j].Path
	})
	return &WorkspaceFileIndex{
		ready:     true,
		files:     copied,
		truncated: truncated,
		maxFiles:  maxFiles,
	}
}

func (idx *WorkspaceFileIndex) Search(query string, limit int) []Candidate {
	return idx.SearchWithRecency(query, limit, nil)
}

func (idx *WorkspaceFileIndex) SearchWithRecency(query string, limit int, recency map[string]int) []Candidate {
	if idx == nil {
		return nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	idx.ensureInitialBuild()
	idx.ensureFreshAsync()
	files := idx.snapshotFiles()
	if len(files) == 0 {
		return nil
	}

	q := strings.ToLower(filepath.ToSlash(strings.TrimSpace(query)))
	type rankedMention struct {
		candidate Candidate
		score     int
		recency   int
	}
	ranked := make([]rankedMention, 0, len(files))
	for _, file := range files {
		score, ok := scoreCandidate(file, q)
		if !ok {
			continue
		}
		recent := 0
		if recency != nil {
			recent = recency[file.Path]
		}
		ranked = append(ranked, rankedMention{
			candidate: file,
			score:     score,
			recency:   recent,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			if ranked[i].recency == ranked[j].recency {
				return ranked[i].candidate.Path < ranked[j].candidate.Path
			}
			return ranked[i].recency > ranked[j].recency
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	out := make([]Candidate, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.candidate)
	}
	return out
}

func (idx *WorkspaceFileIndex) Prewarm() {
	if idx == nil {
		return
	}
	idx.ensureInitialBuild()
}

func (idx *WorkspaceFileIndex) Stats() IndexStats {
	if idx == nil {
		return IndexStats{}
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return IndexStats{
		Count:     len(idx.files),
		MaxFiles:  idx.maxFiles,
		Truncated: idx.truncated,
		Ready:     idx.ready,
	}
}

func (idx *WorkspaceFileIndex) snapshotFiles() []Candidate {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if len(idx.files) == 0 {
		return nil
	}
	out := make([]Candidate, len(idx.files))
	copy(out, idx.files)
	return out
}

func (idx *WorkspaceFileIndex) ensureInitialBuild() {
	if idx == nil {
		return
	}
	idx.mu.RLock()
	ready := idx.ready
	idx.mu.RUnlock()
	if ready {
		return
	}
	idx.rebuildBlocking()
}

func (idx *WorkspaceFileIndex) ensureFreshAsync() {
	if idx == nil {
		return
	}
	idx.mu.RLock()
	stale := idx.shouldRebuildLocked()
	ready := idx.ready
	building := idx.building
	idx.mu.RUnlock()

	if !ready {
		idx.rebuildBlocking()
		return
	}
	if stale && !building {
		go idx.rebuildBlocking()
	}
}

func (idx *WorkspaceFileIndex) rebuildBlocking() {
	if idx == nil {
		return
	}
	idx.mu.Lock()
	if idx.building || !idx.shouldRebuildLocked() {
		idx.mu.Unlock()
		return
	}
	idx.building = true
	root := idx.root
	maxFiles := idx.maxFiles
	idx.mu.Unlock()

	matcher := loadMentionIgnoreMatcher(root)
	files, truncated := buildMentionIndex(root, maxFiles, matcher)

	idx.mu.Lock()
	idx.files = files
	idx.truncated = truncated
	idx.ready = true
	idx.lastBuild = time.Now()
	idx.building = false
	idx.mu.Unlock()
}

func (idx *WorkspaceFileIndex) shouldRebuildLocked() bool {
	if idx == nil {
		return false
	}
	if strings.TrimSpace(idx.root) == "" {
		return !idx.ready
	}
	if !idx.ready || idx.lastBuild.IsZero() {
		return true
	}
	return time.Since(idx.lastBuild) >= mentionIndexRefreshInterval
}

func buildMentionIndex(root string, maxFiles int, matcher mentionIgnoreMatcher) ([]Candidate, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, false
	}
	if maxFiles <= 0 {
		maxFiles = mentionIndexDefaultMaxFiles
	}

	files := make([]Candidate, 0, minInt(maxFiles, 512))
	truncated := false
	_ = filepath.WalkDir(root, func(pathAbs string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if pathAbs == root {
			return nil
		}

		rel, relErr := filepath.Rel(root, pathAbs)
		if relErr != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()

		if d.IsDir() {
			if shouldSkipMentionDir(name) || matcher.SkipDir(name, rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if shouldSkipMentionFile(name) || matcher.SkipFile(name, rel) {
			return nil
		}
		if rel == "" || rel == "." {
			return nil
		}

		files = append(files, Candidate{
			Path:     rel,
			BaseName: filepath.Base(rel),
			TypeTag:  mentionTypeTag(rel),
		})
		if len(files) >= maxFiles {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, truncated
}

func scoreCandidate(file Candidate, query string) (int, bool) {
	path := strings.ToLower(file.Path)
	base := strings.ToLower(file.BaseName)
	switch {
	case query == "":
		return 100, true
	case base == query:
		return 980 - len(path), true
	case strings.HasPrefix(base, query):
		return 900 - len(path), true
	case strings.Contains(base, query):
		return 800 - len(path), true
	case strings.HasPrefix(path, query):
		return 700 - len(path), true
	case strings.Contains(path, query):
		return 600 - len(path), true
	default:
		return 0, false
	}
}

func shouldSkipMentionDir(name string) bool {
	if strings.TrimSpace(name) == "" {
		return true
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, ".") {
		return true
	}
	switch lower {
	case "node_modules", "vendor", "dist", "build":
		return true
	default:
		return false
	}
}

func shouldSkipMentionFile(name string) bool {
	if strings.TrimSpace(name) == "" {
		return true
	}
	return strings.HasPrefix(name, ".")
}

func loadMentionIgnoreMatcher(workspace string) mentionIgnoreMatcher {
	matcher := mentionIgnoreMatcher{
		exact: make(map[string]struct{}, 16),
		globs: make([]string, 0, 8),
	}

	for _, item := range []string{"node_modules", "vendor", "dist", "build"} {
		matcher.addRule(item)
	}
	if env := strings.TrimSpace(os.Getenv("BYTEMIND_MENTION_IGNORE")); env != "" {
		for _, part := range strings.Split(env, ",") {
			matcher.addRule(part)
		}
	}

	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return matcher
	}
	ignorePath := filepath.Join(workspace, ".bytemindignore")
	f, err := os.Open(ignorePath)
	if err != nil {
		return matcher
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		matcher.addRule(scanner.Text())
	}
	return matcher
}

func (m *mentionIgnoreMatcher) addRule(raw string) {
	if m == nil {
		return
	}
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	line = strings.Trim(filepath.ToSlash(line), "/")
	line = strings.ToLower(line)
	if line == "" {
		return
	}
	if strings.ContainsAny(line, "*?[") || strings.Contains(line, "/") {
		m.globs = append(m.globs, line)
		return
	}
	m.exact[line] = struct{}{}
}

func (m mentionIgnoreMatcher) SkipDir(name, relPath string) bool {
	return m.skip(name, relPath)
}

func (m mentionIgnoreMatcher) SkipFile(name, relPath string) bool {
	return m.skip(name, relPath)
}

func (m mentionIgnoreMatcher) skip(name, relPath string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	rel := strings.ToLower(strings.Trim(filepath.ToSlash(strings.TrimSpace(relPath)), "/"))
	if name == "" || rel == "" {
		return false
	}
	if _, ok := m.exact[name]; ok {
		return true
	}
	if _, ok := m.exact[rel]; ok {
		return true
	}
	for _, pattern := range m.globs {
		if ok, _ := path.Match(pattern, rel); ok {
			return true
		}
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

func mentionMaxFilesFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("BYTEMIND_MENTION_MAX_FILES"))
	if raw == "" {
		return mentionIndexDefaultMaxFiles
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return mentionIndexDefaultMaxFiles
	}
	return n
}

func FindActiveToken(input string) (Token, bool) {
	runes := []rune(input)
	if len(runes) == 0 || unicode.IsSpace(runes[len(runes)-1]) {
		return Token{}, false
	}

	start := len(runes) - 1
	for start >= 0 && !unicode.IsSpace(runes[start]) {
		start--
	}
	tokenStart := start + 1
	token := runes[tokenStart:]
	if len(token) == 0 || token[0] != '@' {
		return Token{}, false
	}
	return Token{
		Query: string(token[1:]),
		Start: tokenStart,
		End:   len(runes),
	}, true
}

func InsertIntoInput(input string, token Token, path string) string {
	runes := []rune(input)
	if token.Start < 0 || token.End < token.Start || token.End > len(runes) {
		return input
	}

	mention := []rune("@" + filepath.ToSlash(strings.TrimSpace(path)))
	out := make([]rune, 0, len(runes)+len(mention)+1)
	out = append(out, runes[:token.Start]...)
	out = append(out, mention...)

	if token.End < len(runes) {
		if !unicode.IsSpace(runes[token.End]) {
			out = append(out, ' ')
		}
		out = append(out, runes[token.End:]...)
		return string(out)
	}
	out = append(out, ' ')
	return string(out)
}

func mentionTypeTag(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	switch ext {
	case "go", "md", "txt", "json", "yaml", "yml", "toml", "ini":
		return ext
	case "ts", "tsx", "js", "jsx", "mjs", "cjs":
		return ext
	case "py", "java", "rs", "c", "h", "cpp", "hpp":
		return ext
	case "sh", "bash", "zsh", "fish", "ps1":
		return ext
	case "":
		return "file"
	default:
		if len(ext) <= 4 {
			return ext
		}
		return "file"
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
