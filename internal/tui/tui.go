// Package tui renders APCode's terminal output.
package tui

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"apcode/internal/codeintel"
	"apcode/internal/config"
	projectcontext "apcode/internal/context"
	"apcode/internal/hardware"
	"apcode/internal/model"
	"apcode/internal/recommendation"
)

const logo = ` █████╗ ██████╗  ██████╗ ██████╗ ██████╗ ███████╗
██╔══██╗██╔══██╗██╔════╝██╔═══██╗██╔══██╗██╔════╝
███████║██████╔╝██║     ██║   ██║██║  ██║█████╗
██╔══██║██╔═══╝ ██║     ██║   ██║██║  ██║██╔══╝
██║  ██║██║     ╚██████╗╚██████╔╝██████╔╝███████╗
╚═╝  ╚═╝╚═╝      ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝`

const tagline = `We care about your system. 😄
So you can focus on your ideas. 💡
Making the most of every bit of your laptop. ⚡`

// Banner returns the APCode welcome banner.
func Banner() string {
	return logo + "\n\n" + tagline
}

// PrintWelcome writes the banner, hardware profile, version, and offline
// mode status to w.
func PrintWelcome(w io.Writer, profile hardware.HardwareProfile) {
	fmt.Fprintln(w, Primary(logo))
	fmt.Fprintln(w)
	fmt.Fprintln(w, Muted(tagline))
	fmt.Fprintln(w)
	fmt.Fprintln(w, Box("System", append(hardwareProfileLines(profile),
		fmt.Sprintf("%s %s", Muted("APCode version   :"), config.Version),
		fmt.Sprintf("%s %s", Muted("Offline mode     :"), Success("enabled")),
	)))
}

// hardwareProfileLines renders the hardware profile as styled lines.
func hardwareProfileLines(p hardware.HardwareProfile) []string {
	var lines []string

	cpu := fmt.Sprintf("%s %d", Muted("CPU threads      :"), p.LogicalCPUs)
	if p.PhysicalCoresKnown {
		cpu += fmt.Sprintf(" (%d physical)", p.PhysicalCores)
	}
	lines = append(lines,
		fmt.Sprintf("%s %s", Muted("Operating system :"), p.OS),
		fmt.Sprintf("%s %s", Muted("CPU architecture :"), p.Arch),
		cpu,
	)

	if p.TotalRAMBytes > 0 {
		ram := fmt.Sprintf("%s %s", Muted("Total RAM        :"), formatBytes(p.TotalRAMBytes))
		if p.AvailableRAMKnown && p.AvailableRAMBytes > 0 {
			ram += fmt.Sprintf(" (avail %s)", formatBytes(p.AvailableRAMBytes))
		}
		lines = append(lines, ram)
	}

	if p.GPU.Known {
		gpu := fmt.Sprintf("%s %s", Muted("GPU              :"), p.GPU.Name)
		if p.GPU.Vendor != "" {
			gpu += fmt.Sprintf(" (%s)", p.GPU.Vendor)
		}
		if p.GPU.VRAMKnown && p.GPU.VRAMBytes > 0 {
			gpu += fmt.Sprintf(" - %s VRAM", formatBytes(p.GPU.VRAMBytes))
		}
		lines = append(lines, gpu)
	} else {
		lines = append(lines, fmt.Sprintf("%s %s", Muted("GPU              :"), "unknown"))
	}

	for _, err := range p.DetectionErrors {
		lines = append(lines, fmt.Sprintf("%s %s", Warning("⚠"), Muted(err)))
	}

	return lines
}

// formatBytes formats a byte count as a human-readable string with
// binary (1024-based) units.
func formatBytes(bytes uint64) string {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
		tib = gib * 1024
	)

	switch {
	case bytes >= tib:
		return fmt.Sprintf("%.1f TiB", float64(bytes)/tib)
	case bytes >= gib:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/gib)
	case bytes >= mib:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/mib)
	case bytes >= kib:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/kib)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ProgressBar represents a simple progress bar state.
type ProgressBar struct {
	Label       string
	Progress    float64 // 0.0 to 1.0
	Status      string  // "running", "complete", "failed", "unavailable"
	StatusColor func(string) string
}

// RenderProgressBar renders a progress bar to the writer.
func RenderProgressBar(w io.Writer, pb ProgressBar) {
	const barWidth = 20

	var bar string
	switch pb.Status {
	case "complete":
		bar = Success(strings.Repeat("█", barWidth))
	case "failed":
		bar = Error(strings.Repeat("░", barWidth))
	case "unavailable":
		bar = Warning("unavailable")
	default:
		filled := int(pb.Progress * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		bar = Primary(strings.Repeat("█", filled)) + Muted(strings.Repeat("░", barWidth-filled))
	}

	label := Muted(fmt.Sprintf("%-10s", pb.Label))
	if pb.Status == "running" {
		pct := int(pb.Progress*100 + 0.5)
		fmt.Fprintf(w, "%s %s %3d%%  %s\n", label, bar, pct, pb.StatusColor(pb.Status))
		return
	}
	fmt.Fprintf(w, "%s %s  %s\n", label, bar, pb.StatusColor(pb.Status))
}

// PrintBenchmarkProgress prints the benchmark progress header.
func PrintBenchmarkProgress(w io.Writer) {
	fmt.Fprintln(w, Header("APCode Benchmark"))
	fmt.Fprintln(w)
}

// PrintRecommendation renders the model recommendation results.
func PrintRecommendation(w io.Writer, result recommendation.RecommendationResult) {
	fmt.Fprintln(w, Header("APCode Model Recommendation"))
	fmt.Fprintln(w)

	// Recommended model
	if result.Recommended != nil {
		c := result.Recommended
		fmt.Fprintf(w, "%s %s\n", Success("✓ Recommended:"), Primary(c.Model.Name))
		fmt.Fprintf(w, "  %s %s\n", Muted("ID:"), c.Model.ID)
		fmt.Fprintf(w, "  %s %s (%s)\n", Muted("Provider:"), c.Model.Provider, c.Model.Quantization)
		fmt.Fprintf(w, "  %s %.1fB params, %s\n", Muted("Size:"), c.Model.ParameterCount, formatBytes(c.Model.FileSizeBytes))
		fmt.Fprintf(w, "  %s min %s / rec %s\n", Muted("RAM:"), formatBytes(c.Model.MinimumRAMBytes), formatBytes(c.Model.RecommendedRAMBytes))
		fmt.Fprintf(w, "  %s %s\n", Muted("Context:"), formatContextLength(c.Model.ContextLength))
		fmt.Fprintf(w, "  %s %s\n", Muted("Architecture:"), c.Model.Architecture)
		fmt.Fprintf(w, "  %s %s\n", Muted("Capabilities:"), formatCapabilities(c.Model.Capabilities))
		fmt.Fprintf(w, "  %s %s\n", Muted("Runtimes:"), formatRuntimes(c.Model.RuntimeCompatibility))
		if c.Model.Installed {
			fmt.Fprintf(w, "  %s %s\n", Muted("Status:"), Success("installed locally"))
			if c.Model.InstallPath != "" {
				fmt.Fprintf(w, "  %s %s\n", Muted("Path:"), c.Model.InstallPath)
			}
		} else {
			fmt.Fprintf(w, "  %s %s\n", Muted("Status:"), Warning("not installed"))
		}
		fmt.Fprintf(w, "  %s %d/100\n", Muted("Fit Score:"), c.FitScore)

		if len(c.Reasons) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, Primary("Why this model:"))
			for _, reason := range c.Reasons {
				fmt.Fprintf(w, "  %s %s\n", Success("•"), reason)
			}
		}
		if len(c.Warnings) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, Primary("Warnings:"))
			for _, warn := range c.Warnings {
				fmt.Fprintf(w, "  %s %s\n", Warning("⚠"), warn)
			}
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, Rule())
	fmt.Fprintln(w)

	// Other candidates
	if len(result.Candidates) > 1 {
		fmt.Fprintln(w, Primary("Other candidates:"))
		fmt.Fprintln(w)
		for i, c := range result.Candidates {
			if i == 0 {
				continue // Skip recommended
			}
			status := Muted("available")
			if c.Model.Installed {
				status = Success("installed")
			}
			fmt.Fprintf(w, "  %d. %s %s (score: %d) %s\n",
				i, c.Model.Name, Muted("("+c.Model.ID+")"), c.FitScore, status)
			fmt.Fprintf(w, "     %s %.1fB params, %s\n", Muted("Size:"), c.Model.ParameterCount, formatBytes(c.Model.FileSizeBytes))
			fmt.Fprintf(w, "     %s min %s / rec %s\n", Muted("RAM:"), formatBytes(c.Model.MinimumRAMBytes), formatBytes(c.Model.RecommendedRAMBytes))
			fmt.Fprintf(w, "     %s %s\n", Muted("Context:"), formatContextLength(c.Model.ContextLength))
			if len(c.Reasons) > 0 {
				fmt.Fprintf(w, "     %s %s\n", Muted("Reasons:"), joinWithComma(c.Reasons))
			}
			if len(c.Warnings) > 0 {
				fmt.Fprintf(w, "     %s %s\n", Muted("Warnings:"), joinWithComma(c.Warnings))
			}
			fmt.Fprintln(w)
		}
	}

	// Rejected models
	if len(result.Rejected) > 0 {
		fmt.Fprintln(w, Primary("Rejected (incompatible):"))
		fmt.Fprintln(w)
		for _, c := range result.Rejected {
			fmt.Fprintf(w, "  %s %s %s\n", Error("✗"), c.Model.Name, Muted("("+c.Model.ID+")"))
			if c.RejectionReason != "" {
				fmt.Fprintf(w, "     %s %s\n", Muted("Reason:"), c.RejectionReason)
			}
			fmt.Fprintln(w)
		}
	}

	// Uncertainty
	if result.Uncertainty != "" && result.Uncertainty != "No significant uncertainties" {
		fmt.Fprintln(w, Rule())
		fmt.Fprintln(w)
		fmt.Fprintln(w, Primary("Uncertainty:"))
		fmt.Fprintf(w, "  %s\n", Warning(result.Uncertainty))
		fmt.Fprintln(w)
	}

	// Summary
	fmt.Fprintf(w, "%s %d evaluated, %d compatible, %d rejected\n",
		Muted("Summary:"), len(result.Candidates)+len(result.Rejected), len(result.Candidates), len(result.Rejected))
	if result.BenchmarkRun {
		fmt.Fprintf(w, "%s %s\n", Muted("Benchmark:"), Success("used for ranking"))
	} else {
		fmt.Fprintf(w, "%s %s\n", Muted("Benchmark:"), Warning("not run (use --benchmark for better accuracy)"))
	}
	if result.AvailableRAMKnown {
		fmt.Fprintf(w, "%s %s\n", Muted("Available RAM:"), Success("measured"))
	} else {
		fmt.Fprintf(w, "%s %s\n", Muted("Available RAM:"), Warning("estimated from total"))
	}
}

func formatContextLength(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM tokens", float64(tokens)/1000000)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fK tokens", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d tokens", tokens)
}

func formatCapabilities(caps model.Capabilities) string {
	var names []string
	for _, c := range caps {
		names = append(names, string(c))
	}
	return joinWithComma(names)
}

func formatRuntimes(runtimes []model.Runtime) string {
	var names []string
	for _, r := range runtimes {
		names = append(names, string(r))
	}
	return joinWithComma(names)
}

func joinWithComma(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0]
	}
	result := items[0]
	for _, item := range items[1:] {
		result += ", " + item
	}
	return result
}

// PrintContext renders project context discovery results.
func PrintContext(w io.Writer, result *projectcontext.Result) {
	fmt.Fprintln(w, Header("APCode Project Context"))
	fmt.Fprintln(w)

	// Project root
	relRoot := result.Root
	if cwd, err := filepath.Abs("."); err == nil {
		if rel, err := filepath.Rel(cwd, result.Root); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			relRoot = rel
		}
	}
	fmt.Fprintf(w, "%s %s\n", Muted("Project:"), Primary(relRoot))
	fmt.Fprintf(w, "%s %s\n", Muted("Root:"), result.Root)
	fmt.Fprintln(w)

	// Files discovered / ignored
	fmt.Fprintf(w, "%s %d\n", Muted("Files discovered:"), len(result.Files))
	fmt.Fprintf(w, "%s %d\n", Muted("Files ignored:"), len(result.Ignored))
	if result.Truncated {
		fmt.Fprintf(w, "%s %s\n", Muted("Truncated:"), Warning("yes (budget or file limit)"))
	}
	fmt.Fprintln(w)

	// Show discovered files (up to 20)
	if len(result.Files) > 0 {
		fmt.Fprintln(w, Primary("Discovered files:"))
		limit := 20
		if len(result.Files) < limit {
			limit = len(result.Files)
		}
		for i := 0; i < limit; i++ {
			f := result.Files[i]
			fmt.Fprintf(w, "  %s %s %s (%s, %s, ~%d tokens)\n",
				Success("•"), f.Path, Muted("["+f.Language+"]"), formatBytes(uint64(f.Size)), formatContextLength(f.Lines), f.Tokens)
		}
		if len(result.Files) > limit {
			fmt.Fprintf(w, "  %s %s\n", Muted("... and"), Muted(fmt.Sprintf("%d more", len(result.Files)-limit)))
		}
		fmt.Fprintln(w)
	}

	// Files ignored breakdown (top 10)
	if len(result.Ignored) > 0 {
		fmt.Fprintln(w, Primary("Ignored files:"))
		limit := 10
		if len(result.Ignored) < limit {
			limit = len(result.Ignored)
		}
		for i := 0; i < limit; i++ {
			ig := result.Ignored[i]
			fmt.Fprintf(w, "  %s %s %s\n", Warning("•"), ig.Path, Muted("("+ig.Reason+")"))
		}
		if len(result.Ignored) > limit {
			fmt.Fprintf(w, "  %s %s\n", Muted("... and"), Muted(fmt.Sprintf("%d more ignored", len(result.Ignored)-limit)))
		}
		fmt.Fprintln(w)
	}

	// Languages
	if len(result.Languages) > 0 {
		fmt.Fprintln(w, Primary("Languages detected:"))
		// Sort by count desc
		type kv struct {
			lang  string
			count int
		}
		var kvs []kv
		for k, v := range result.Languages {
			kvs = append(kvs, kv{k, v})
		}
		sort.Slice(kvs, func(i, j int) bool {
			if kvs[i].count != kvs[j].count {
				return kvs[i].count > kvs[j].count
			}
			return kvs[i].lang < kvs[j].lang
		})
		for _, kv := range kvs {
			fmt.Fprintf(w, "  %s %s: %d file(s)\n", Success("•"), kv.lang, kv.count)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "%s %s\n", Muted("Languages detected:"), Muted("none"))
		fmt.Fprintln(w)
	}

	// Estimated context size
	fmt.Fprintln(w, Primary("Estimated context size:"))
	fmt.Fprintf(w, "  %s %s\n", Muted("Total size:"), formatBytes(uint64(result.TotalSize)))
	fmt.Fprintf(w, "  %s %d tokens (~%s)\n", Muted("Tokens:"), result.TotalTokens, formatBytes(uint64(result.TotalTokens*4)))
	if len(result.Selected) > 0 && len(result.Selected) != len(result.Files) {
		fmt.Fprintf(w, "  %s %d tokens (%d files selected)\n", Muted("Selected:"), result.SelectedTokens, len(result.Selected))
	}
	fmt.Fprintln(w)

	// Budget note
	fmt.Fprintf(w, "%s %s\n", Muted("Budget:"), Muted("respected (no blind full-repo load)"))
}

// PrintSearchResults renders codeintel search results.
func PrintSearchResults(w io.Writer, query, dir string, symbols []codeintel.Symbol, results []codeintel.SearchResult, idx *codeintel.Index) {
	fmt.Fprintln(w, Header("APCode Search"))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s %q\n", Muted("Query:"), Primary(query))
	fmt.Fprintf(w, "%s %s\n", Muted("Directory:"), dir)
	fmt.Fprintln(w)

	// Summary: languages, files, symbols
	files := idx.SnapshotFiles()
	fmt.Fprintf(w, "%s %d file(s) indexed\n", Muted("Files:"), len(files))
	// Count languages
	langCount := make(map[codeintel.Language]int)
	for _, f := range files {
		langCount[f.Language]++
	}
	if len(langCount) > 0 {
		fmt.Fprintf(w, "%s ", Muted("Languages:"))
		first := true
		for lang, cnt := range langCount {
			if !first {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%s (%d)", lang, cnt)
			first = false
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%s %d symbol(s) discovered\n", Muted("Symbols:"), len(idx.SnapshotSymbols()))
	fmt.Fprintf(w, "%s %d import(s)\n", Muted("Imports:"), len(idx.SnapshotImports()))
	fmt.Fprintf(w, "%s %d relationship(s)\n", Muted("Relationships:"), len(idx.SnapshotRelationships()))
	fmt.Fprintln(w)

	if len(symbols) > 0 {
		fmt.Fprintln(w, Primary("Symbol matches:"))
		for _, s := range symbols {
			fmt.Fprintf(w, "  %s %s %s %s:%d [%s] %s\n",
				Success("•"),
				s.Name,
				Muted("("+string(s.Kind)+")"),
				s.Path,
				s.Line,
				s.Language,
				Muted(s.Signature))
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "%s no exact symbol match for %q\n", Muted("Symbols:"), query)
		fmt.Fprintln(w)
	}

	if len(results) == 0 {
		fmt.Fprintf(w, "%s no results for %q\n", Warning("No results"), query)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s try a broader query or check --dir\n", Muted("Hint:"))
		return
	}

	fmt.Fprintf(w, "%s %d hit(s):\n", Primary("Search results:"), len(results))
	for i, r := range results {
		preview := r.Preview
		// Highlight query within preview (simple)
		if preview != "" {
			// Keep as is; color highlight?
			preview = strings.ReplaceAll(preview, query, Primary(query))
			// Case-insensitive highlight not perfect but okay
		}
		symInfo := ""
		if r.Symbol != nil {
			symInfo = Muted(fmt.Sprintf(" [%s:%s]", r.Symbol.Kind, r.Symbol.Name))
		}
		fmt.Fprintf(w, "  %d. %s:%d:%d %s\n", i+1, r.Path, r.Line, r.Column, symInfo)
		if preview != "" {
			fmt.Fprintf(w, "     %s\n", Muted(preview))
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s search is local, offline, and LLM-free (regex + file walk)\n", Muted("Note:"))
}
