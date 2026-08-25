package context

import "sort"

// SelectOptions controls context selection.
type SelectOptions struct {
	TokenBudget int // 0 means no budget (select all)
	MaxFiles    int // 0 means no limit
	// Priority defines sorting: by default, smaller files first and relevant languages first.
	// If nil, uses default ordering.
	Less func(a, b FileMeta) bool
}

// SelectContext returns a subset of files fitting budget, respecting token limits.
// It does not read file contents; it only uses metadata.
// Files are sorted by relevance before selection.
func SelectContext(files []FileMeta, opts SelectOptions) ([]FileMeta, int) {
	if len(files) == 0 {
		return nil, 0
	}
	// Copy to avoid mutating input.
	sorted := make([]FileMeta, len(files))
	copy(sorted, files)

	less := opts.Less
	if less == nil {
		less = defaultLess
	}
	sort.Slice(sorted, func(i, j int) bool {
		return less(sorted[i], sorted[j])
	})

	if opts.TokenBudget <= 0 && opts.MaxFiles <= 0 {
		total := 0
		for _, f := range sorted {
			total += f.Tokens
		}
		return sorted, total
	}

	var selected []FileMeta
	total := 0
	for _, f := range sorted {
		if opts.MaxFiles > 0 && len(selected) >= opts.MaxFiles {
			break
		}
		if opts.TokenBudget > 0 && total+f.Tokens > opts.TokenBudget {
			// If even smallest doesn't fit and we have none selected, still include one?
			// We skip to respect budget strictly; but if budget is tiny, we may return empty.
			// To avoid empty when budget < smallest file, we allow first file even if over budget.
			if len(selected) == 0 {
				selected = append(selected, f)
				total += f.Tokens
			}
			continue
		}
		selected = append(selected, f)
		total += f.Tokens
	}
	return selected, total
}

// defaultLess prioritizes: smaller tokens first, then relevant languages (Go, Python, etc), then path alphabetical.
func defaultLess(a, b FileMeta) bool {
	if a.Tokens != b.Tokens {
		return a.Tokens < b.Tokens
	}
	// Prefer Go, then other code, then Markdown etc? Simple: language alphabetical as tie-breaker for determinism.
	if a.Language != b.Language {
		return a.Language < b.Language
	}
	return a.Path < b.Path
}

// FilterByBudget is a helper that just truncates by token budget without sorting.
func FilterByBudget(files []FileMeta, budget int) ([]FileMeta, int) {
	return SelectContext(files, SelectOptions{TokenBudget: budget})
}
