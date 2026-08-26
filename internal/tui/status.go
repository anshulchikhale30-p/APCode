package tui

import (
	"fmt"
	"io"
	"strings"
)

// StatusData is everything the /status screen renders. The caller (CLI
// layer) gathers real values; this package only formats them.
type StatusData struct {
	Version string

	ProjectLanguage string // "Go", "Python", ... ("unknown" allowed)
	ProjectFiles    int
	GitRepo         bool
	GitBranch       string
	GitChanges      string // free-form, e.g. "3 modified"; may be empty

	OS  string
	CPU string // e.g. "12 threads"
	RAM string // e.g. "15.3 GiB"
	GPU string // may be empty/unknown

	RuntimeName  string // e.g. "llama.cpp", "native"
	RuntimeModel string
	RuntimeState string // "Ready", "Unavailable", ...
	RuntimeReady bool
}

// RenderStatus writes the polished /status screen:
//
//	APCode Status
//
//	PROJECT
//	  Language       Go
//	  Files          42
//	  Git            main
//
//	SYSTEM
//	  OS             Windows
//	  CPU            12 threads
//	  RAM            15.3 GiB
//	  GPU            AMD Radeon
//
//	RUNTIME
//	  Runtime        llama.cpp
//	  Model          qwen2.5-coder
//	  Status         Ready
func RenderStatus(w io.Writer, d StatusData) {
	fmt.Fprintln(w, Bold("APCode Status"))
	if d.Version != "" {
		fmt.Fprintf(w, "%s\n", Muted("v"+d.Version))
	}
	fmt.Fprintln(w)

	label := func(s string) string { return Muted(padRight(s, 15)) }

	fmt.Fprintln(w, Primary("PROJECT"))
	if d.ProjectLanguage != "" {
		fmt.Fprintf(w, "  %s%s\n", label("Language"), d.ProjectLanguage)
	}
	if d.ProjectFiles > 0 {
		fmt.Fprintf(w, "  %s%d\n", label("Files"), d.ProjectFiles)
	}
	git := "not initialized"
	if d.GitRepo {
		git = d.GitBranch
		if git == "" {
			git = "yes"
		}
	}
	fmt.Fprintf(w, "  %s%s\n", label("Git"), git)
	if d.GitChanges != "" {
		fmt.Fprintf(w, "  %s%s\n", label("Changes"), d.GitChanges)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, Primary("SYSTEM"))
	if d.OS != "" {
		fmt.Fprintf(w, "  %s%s\n", label("OS"), d.OS)
	}
	if d.CPU != "" {
		fmt.Fprintf(w, "  %s%s\n", label("CPU"), d.CPU)
	}
	if d.RAM != "" {
		fmt.Fprintf(w, "  %s%s\n", label("RAM"), d.RAM)
	}
	gpu := d.GPU
	if gpu == "" {
		gpu = "unknown"
	}
	fmt.Fprintf(w, "  %s%s\n", label("GPU"), gpu)
	fmt.Fprintln(w)

	fmt.Fprintln(w, Primary("RUNTIME"))
	rt := d.RuntimeName
	if rt == "" {
		rt = "none detected"
	}
	fmt.Fprintf(w, "  %s%s\n", label("Runtime"), rt)
	mdl := d.RuntimeModel
	if mdl == "" {
		mdl = "no local model installed"
	}
	fmt.Fprintf(w, "  %s%s\n", label("Model"), mdl)
	state := d.RuntimeState
	if state == "" {
		state = "Unknown"
	}
	if d.RuntimeReady {
		fmt.Fprintf(w, "  %s%s\n", label("Status"), Success(state))
	} else {
		fmt.Fprintf(w, "  %s%s\n", label("Status"), Warning(state))
	}
}

// ModelIndicator builds the bottom-of-screen model line from real runtime
// state. It never claims a model exists when it does not.
func ModelIndicator(runtimeName, modelName string) string {
	switch {
	case runtimeName != "" && modelName != "":
		return runtimeName + " · " + modelName
	case modelName != "":
		return "Local · " + modelName
	case runtimeName != "":
		return runtimeName + " · no model installed"
	default:
		return GlyphWarning + " No local model installed"
	}
}

// ProjectLine builds the compact one-line project summary shown on the
// welcome screen: "Go · 42 files · Git: main".
func ProjectLine(language string, files int, gitRepo bool, branch string) string {
	parts := []string{}
	if language != "" && language != "unknown" {
		parts = append(parts, language)
	}
	if files > 0 {
		parts = append(parts, fmt.Sprintf("%d file%s", files, plural(files)))
	}
	if gitRepo {
		b := branch
		if b == "" {
			b = "clean"
		}
		parts = append(parts, "Git: "+b)
	} else if len(parts) > 0 {
		parts = append(parts, "Git: not initialized")
	}
	if len(parts) == 0 {
		return "Empty directory"
	}
	return strings.Join(parts, " · ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
