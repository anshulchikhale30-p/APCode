package codeintel

import (
	"bytes"
	"sort"
	"strings"
)

// SearchResult is a single hit for a search query.
type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Preview string `json:"preview"`
	// Language of the file
	Language Language `json:"language"`
	// Symbol associated if line is a symbol definition
	Symbol *Symbol `json:"symbol,omitempty"`
	// Score for ranking (higher is better)
	Score int `json:"score"`
}

// Search performs a full-text search over indexed files.
// Lightweight ranking: filename matches score higher than content matches.
func (idx *Index) Search(query string) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	qLower := strings.ToLower(q)
	var results []SearchResult
	// Build symbol map by path:line for quick lookup
	symByPathLine := make(map[string]*Symbol)
	for i := range idx.Symbols {
		s := &idx.Symbols[i]
		key := s.Path + ":" + itoa(s.Line)
		symByPathLine[key] = s
	}
	for _, fi := range idx.Files {
		rel := fi.Path
		content, ok := idx.contents[rel]
		if !ok {
			continue
		}
		// Filename match?
		filenameScore := 0
		if strings.Contains(strings.ToLower(rel), qLower) {
			filenameScore = 100
			// Exact basename higher
			if strings.EqualFold(rel, q) || strings.EqualFold(strings.TrimSuffix(rel, ".go"), q) {
				filenameScore = 200
			}
		}
		lines := bytes.Split(content, []byte{'\n'})
		for i, line := range lines {
			lineno := i + 1
			lineStr := string(line)
			lowerLine := strings.ToLower(lineStr)
			if !strings.Contains(lowerLine, qLower) && filenameScore == 0 {
				continue
			}
			score := 0
			col := strings.Index(lowerLine, qLower)
			if col >= 0 {
				score = 10
				// Boost if symbol definition line
				key := rel + ":" + itoa(lineno)
				if _, isSym := symByPathLine[key]; isSym {
					score += 20
				}
				// Boost if line starts with match (e.g., function name at start)
				trim := strings.TrimSpace(lowerLine)
				if strings.HasPrefix(trim, qLower) {
					score += 10
				}
				// Penalize very long lines slightly
				if len(lineStr) > 200 {
					score -= 2
				}
				col++ // 1-indexed
			} else {
				col = 1
			}
			// Filename bonus propagates to all lines? But to avoid spamming, only for first line if file name matches
			if filenameScore > 0 {
				if lineno == 1 || strings.Contains(lowerLine, qLower) {
					score += filenameScore
					if col < 0 {
						col = 1
					}
				} else {
					// For files matched only by name, include one synthetic result? We already handle content.
					// To avoid duplicate, only add file-level result once; we add it below.
					continue
				}
			}
			if score == 0 && filenameScore == 0 {
				continue
			}
			preview := strings.TrimSpace(lineStr)
			if len(preview) > 120 {
				preview = preview[:120]
			}
			// Find associated symbol if any
			key := rel + ":" + itoa(lineno)
			var sym *Symbol
			if s, ok := symByPathLine[key]; ok {
				cp := *s
				sym = &cp
			}
			results = append(results, SearchResult{
				Path:     rel,
				Line:     lineno,
				Column:   col,
				Preview:  preview,
				Language: fi.Language,
				Symbol:   sym,
				Score:    score,
			})
		}
		// If filename matched but no content matched, add a file-level result
		if filenameScore > 0 {
			has := false
			for _, r := range results {
				if r.Path == rel {
					has = true
					break
				}
			}
			if !has {
				results = append(results, SearchResult{
					Path:     rel,
					Line:     1,
					Column:   1,
					Preview:  rel,
					Language: fi.Language,
					Score:    filenameScore,
				})
			}
		}
	}
	// Rank: score desc, then path asc, line asc
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Path != results[j].Path {
			return results[i].Path < results[j].Path
		}
		return results[i].Line < results[j].Line
	})
	// Cap to 200 results for practicality
	if len(results) > 200 {
		results = results[:200]
	}
	return results, nil
}

// SimpleSearch is a convenience for one-off search without building full index.
// It walks dir and searches directly; used for CLI when index not needed separately.
func SimpleSearch(dir, query string) ([]SearchResult, error) {
	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		return nil, err
	}
	return idx.Search(query)
}
