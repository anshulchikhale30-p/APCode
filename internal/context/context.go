// Package context defines how APCode will gather project context
// (files, symbols, history) to feed into prompts.
package context

import (
	stdcontext "context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNotImplemented is returned by placeholder context gathering.
var ErrNotImplemented = errors.New("context: not implemented")

// Provider assembles relevant project context for a task.
type Provider interface {
	// Gather returns context material relevant to the query.
	Gather(query string) ([]byte, error)
	// GatherWithContext returns context material respecting cancellation.
	GatherWithContext(ctx stdcontext.Context, query string) ([]byte, error)
}

// ProjectProvider is a concrete Provider that uses file walking and selection.
type ProjectProvider struct {
	Root   string
	Config Config
	Result *Result
}

// NewProvider creates a provider for the project at root.
// If root is empty, DetectProjectRoot is used.
func NewProvider(root string, cfg Config) (*ProjectProvider, error) {
	if root == "" {
		var err error
		root, err = DetectProjectRoot(".")
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	cfg.Root = abs
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = 512 * 1024
	}
	if cfg.MaxTotalFiles == 0 {
		cfg.MaxTotalFiles = 10000
	}
	return &ProjectProvider{Root: abs, Config: cfg}, nil
}

// Gather implements Provider.
func (p *ProjectProvider) Gather(query string) ([]byte, error) {
	return p.GatherWithContext(stdcontext.Background(), query)
}

// GatherWithContext gathers context respecting token budgets and selection.
func (p *ProjectProvider) GatherWithContext(ctx stdcontext.Context, query string) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	// Ensure result is populated.
	if p.Result == nil {
		res, err := WalkProject(p.Root, p.Config)
		if err != nil {
			return nil, err
		}
		p.Result = res
	}
	// If query is empty, return selected files metadata as context.
	// For query filtering, we do simple substring match on path.
	var candidates []FileMeta
	if strings.TrimSpace(query) == "" {
		candidates = p.Result.Selected
		if len(candidates) == 0 {
			candidates = p.Result.Files
		}
	} else {
		q := strings.ToLower(query)
		for _, f := range p.Result.Files {
			if strings.Contains(strings.ToLower(f.Path), q) {
				candidates = append(candidates, f)
			}
		}
		// If no candidates match query, fallback to selected
		if len(candidates) == 0 {
			candidates = p.Result.Selected
		}
		// Re-apply budget to filtered candidates
		if p.Config.TokenBudget > 0 {
			sel, _ := SelectContext(candidates, SelectOptions{TokenBudget: p.Config.TokenBudget})
			candidates = sel
		}
	}
	// Avoid loading entire repo blindly: only read files up to budget and size.
	// We return concatenated metadata, not full file contents, unless files are small.
	var out []byte
	var total int64
	for _, f := range candidates {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
		if f.Size > p.Config.MaxFileSize {
			continue
		}
		content, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		// Enforce token budget on the fly.
		est := EstimateTokens(int64(len(content)))
		if p.Config.TokenBudget > 0 && total+int64(est) > int64(p.Config.TokenBudget) && len(out) > 0 {
			break
		}
		// Prefix with file header
		header := "--- " + f.Path + " (" + f.Language + ") ---\n"
		out = append(out, []byte(header)...)
		out = append(out, content...)
		out = append(out, '\n')
		total += int64(est)
		if p.Config.TokenBudget > 0 && int(total) >= p.Config.TokenBudget {
			break
		}
	}
	return out, nil
}

// GetResult returns the last Walk result, or walks if not yet done.
func (p *ProjectProvider) GetResult() (*Result, error) {
	if p.Result != nil {
		return p.Result, nil
	}
	res, err := WalkProject(p.Root, p.Config)
	if err != nil {
		return nil, err
	}
	p.Result = res
	return res, nil
}

// ListLanguages returns sorted language stats.
func (r *Result) ListLanguages() []string {
	if r == nil || r.Languages == nil {
		return nil
	}
	type kv struct {
		lang  string
		count int
	}
	var kvs []kv
	for k, v := range r.Languages {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].count != kvs[j].count {
			return kvs[i].count > kvs[j].count
		}
		return kvs[i].lang < kvs[j].lang
	})
	var out []string
	for _, kv := range kvs {
		out = append(out, kv.lang)
	}
	return out
}

// EstimatedContextSize returns total tokens and human readable size.
func (r *Result) EstimatedContextSize() (tokens int, size int64) {
	if r == nil {
		return 0, 0
	}
	if len(r.Selected) > 0 {
		return r.SelectedTokens, r.TotalSize
	}
	return r.TotalTokens, r.TotalSize
}
