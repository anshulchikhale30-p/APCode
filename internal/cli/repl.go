// Package cli implements the interactive REPL for APCode.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"apcode/internal/config"
	projectcontext "apcode/internal/context"
	"apcode/internal/hardware"
	"apcode/internal/localmodel"
	"apcode/internal/model"
	"apcode/internal/runtime"
	"apcode/internal/tools"
	"apcode/internal/tui"
)

// REPL is the interactive terminal session.
type REPL struct {
	In          io.Reader
	Out         io.Writer
	ErrOut      io.Writer
	Workspace   string
	IsGitRepo   bool
	GitBranch   string
	ProjectCtx  *projectcontext.Result
	Hardware    hardware.HardwareProfile
	Runtime     runtime.InferenceRuntime
	RuntimeName string
	Model       *model.ModelMetadata
	ModelDir    string
	Registry    *tools.Registry
	History     []Message
	Journal     *Journal
	NoColor     bool
	lastPlan    string
	reader      *bufio.Reader
}

// Message is a conversation turn.
type Message struct {
	Role    string
	Content string
}

// NewREPL creates a REPL with detected workspace and runtime.
func NewREPL(in io.Reader, out, errOut io.Writer) (*REPL, error) {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}
	ws, _ := os.Getwd()
	if ws == "" {
		ws = "."
	}
	absWs, _ := filepath.Abs(ws)

	isGit := false
	branch := ""
	if _, err := os.Stat(filepath.Join(absWs, ".git")); err == nil {
		isGit = true
		if data, err := os.ReadFile(filepath.Join(absWs, ".git", "HEAD")); err == nil {
			s := strings.TrimSpace(string(data))
			if strings.HasPrefix(s, "ref: refs/heads/") {
				branch = strings.TrimPrefix(s, "ref: refs/heads/")
			} else if len(s) >= 7 {
				branch = s[:7]
			}
		}
		if branch == "" {
			branch = "main"
		}
	} else {
		dir := absWs
		for dir != filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				isGit = true
				break
			}
			dir = filepath.Dir(dir)
		}
	}

	var projResult *projectcontext.Result
	if root, err := projectcontext.DetectProjectRoot(absWs); err == nil {
		cfg := projectcontext.DefaultConfig()
		cfg.Root = root
		if res, err := projectcontext.WalkProject(root, cfg); err == nil {
			projResult = res
		}
	}

	hw, _ := hardware.Detect()

	session := ResolveSession()
	rt := session.Runtime
	rtName := session.RuntimeName
	mdl := session.Model
	modelDir := session.ModelDir

	reg, _ := tools.DefaultRegistryWithWorkspace(absWs)
	if reg == nil {
		reg = tools.DefaultRegistry()
	}
	tools.RegisterSpecTools(reg, absWs)

	journal := NewJournal()
	if wrapped, werr := wrapRegistryWithJournal(reg, absWs, journal); werr == nil {
		reg = wrapped
	}

	return &REPL{
		In:          in,
		Out:         out,
		ErrOut:      errOut,
		Workspace:   absWs,
		IsGitRepo:   isGit,
		GitBranch:   branch,
		ProjectCtx:  projResult,
		Hardware:    hw,
		Runtime:     rt,
		RuntimeName: rtName,
		Model:       mdl,
		ModelDir:    modelDir,
		Registry:    reg,
		History:     []Message{},
		Journal:     journal,
		reader:      bufio.NewReader(in),
	}, nil
}

// Run starts the interactive loop.
func (r *REPL) Run(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	r.printBanner()

	if ctx == nil {
		ctx = context.Background()
	}
	if r.reader == nil {
		r.reader = bufio.NewReader(r.In)
	}
	reader := r.reader

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(r.Out, "\n"+tui.Muted("Session cancelled."))
			return ctx.Err()
		default:
		}

		fmt.Fprint(r.Out, tui.Primary("You")+" "+tui.Muted("❯ "))

		lineCh := make(chan string, 1)
		errCh := make(chan error, 1)
		go func() {
			line, err := reader.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
			lineCh <- line
		}()

		select {
		case <-sigCh:
			fmt.Fprintln(r.Out, "^C")
			fmt.Fprintln(r.Out, tui.Muted("(Ctrl+C) - type /exit to quit"))
			continue
		case err := <-errCh:
			if err == io.EOF {
				fmt.Fprintln(r.Out, "\n"+tui.Muted("Goodbye!"))
				return nil
			}
			fmt.Fprintf(r.ErrOut, "read error: %v\n", err)
			continue
		case line := <-lineCh:
			input := strings.TrimSpace(line)
			if input == "" {
				continue
			}
			if strings.HasPrefix(input, "/") {
				shouldExit := r.handleSlashCommand(ctx, input)
				if shouldExit {
					fmt.Fprintln(r.Out, tui.Muted("Goodbye!"))
					return nil
				}
				continue
			}
			r.History = append(r.History, Message{Role: "user", Content: input})
			r.Journal.BeginGroup()
			agentCtx, cancel := context.WithCancel(ctx)
			go func() {
				select {
				case <-sigCh:
					fmt.Fprintln(r.Out, "\n"+tui.Muted("Interrupted. (Ctrl+C)"))
					cancel()
					if r.Runtime != nil {
						_ = r.Runtime.Cancel(context.Background())
					}
				case <-agentCtx.Done():
					// Normal completion or upstream cancellation - stop watching.
				}
			}()
			response, err := r.runAgent(agentCtx, input)
			cancel()
			r.Journal.EndGroup()
			if err != nil {
				if err == context.Canceled {
					fmt.Fprintln(r.Out, tui.Muted("Cancelled."))
				} else {
					fmt.Fprintf(r.Out, "%s %v\n", tui.Error("Error:"), err)
				}
				continue
			}
			r.History = append(r.History, Message{Role: "assistant", Content: response})
			fmt.Fprintf(r.Out, "\n%s %s\n\n", tui.Secondary("APCode")+" "+tui.Muted("›"), response)
		}
	}
}

func (r *REPL) printBanner() {
	fmt.Fprintln(r.Out, tui.Primary(tui.Banner()))
	fmt.Fprintln(r.Out)
	status := tui.Success("✓")
	if r.ProjectCtx != nil {
		lang := "unknown"
		if len(r.ProjectCtx.Languages) > 0 {
			max := 0
			for l, c := range r.ProjectCtx.Languages {
				if c > max {
					max = c
					lang = l
				}
			}
		}
		fmt.Fprintf(r.Out, "%s %s\n", status, fmt.Sprintf("Project detected (%s, %d files)", lang, len(r.ProjectCtx.Files)))
	} else {
		fmt.Fprintf(r.Out, "%s %s\n", tui.Warning("○"), "No project detected")
	}
	if r.IsGitRepo {
		branchInfo := ""
		if r.GitBranch != "" {
			branchInfo = fmt.Sprintf(" (%s)", r.GitBranch)
		}
		fmt.Fprintf(r.Out, "%s %s\n", status, "Git repository detected"+branchInfo)
	} else {
		fmt.Fprintf(r.Out, "%s %s\n", tui.Muted("○"), "Not a git repository")
	}
	if r.Runtime != nil {
		avail := runtime.IsAvailable(context.Background(), r.Runtime)
		if avail {
			name := r.RuntimeName
			if name == "" {
				name = string(r.Runtime.Type())
			}
			fmt.Fprintf(r.Out, "%s %s\n", status, fmt.Sprintf("Runtime ready (%s)", name))
		} else {
			fmt.Fprintf(r.Out, "%s %s\n", tui.Warning("○"), "Runtime not available")
		}
	} else {
		fmt.Fprintf(r.Out, "%s %s\n", tui.Warning("○"), "No runtime detected")
	}
	if r.Model != nil {
		fmt.Fprintf(r.Out, "%s %s\n", status, fmt.Sprintf("Local model: %s (%s)", r.Model.Name, r.Model.ID))
		// Try to load to show "Model loaded"
		// Memory safety check before loading
		hw, _ := hardware.Detect()
		availableRAM := hw.TotalRAMBytes
		if hw.AvailableRAMKnown && hw.AvailableRAMBytes > 0 {
			availableRAM = hw.AvailableRAMBytes
		}
		if availableRAM > 0 && availableRAM < r.Model.MinimumRAMBytes {
			fmt.Fprintf(r.Out, "%s %s (requires %s, available %s)\n", tui.Warning("⚠"), "Insufficient RAM for model", formatBytes(r.Model.MinimumRAMBytes), formatBytes(availableRAM))
		} else {
			// Try loading
			ctxLoad, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := r.Runtime.Load(ctxLoad, r.Model)
			cancel()
			if err == nil {
				fmt.Fprintf(r.Out, "%s %s\n", status, "Model loaded")
				_ = r.Runtime.Unload(context.Background())
			} else {
				fmt.Fprintf(r.Out, "%s %s: %v\n", tui.Warning("○"), "Model not loaded", err)
			}
		}
	} else {
		fmt.Fprintf(r.Out, "%s %s\n", tui.Warning("○"), "No local model installed")
		fmt.Fprintf(r.Out, "  %s\n", tui.Muted("Use: apcode models"))
	}
	fmt.Fprintln(r.Out)
	fmt.Fprintln(r.Out, tui.Muted(fmt.Sprintf("APCode v%s · offline-first · type /help for commands", config.Version)))
	fmt.Fprintln(r.Out)
}

func (r *REPL) handleSlashCommand(ctx context.Context, input string) bool {
	cmd := strings.ToLower(strings.TrimSpace(input))
	parts := strings.Fields(cmd)
	base := parts[0]
	switch base {
	case "/help", "/h", "/?":
		r.printHelp()
	case "/clear", "/cls":
		fmt.Fprint(r.Out, "\033[H\033[2J")
		r.printBanner()
	case "/context", "/ctx":
		r.handleContext()
	case "/models":
		r.handleModels()
	case "/runtime", "/rt":
		r.handleRuntime()
	case "/diff":
		r.handleDiff(ctx)
	case "/git":
		r.handleDiff(ctx)
		r.handleStatus()
	case "/status", "/st":
		r.handleStatus()
	case "/files":
		r.handleFiles()
	case "/search":
		r.handleSearch(ctx, input)
	case "/model":
		r.handleModel()
	case "/plan":
		r.handlePlan()
	case "/compact":
		r.handleCompact()
	case "/permissions":
		r.handlePermissions()
	case "/rollback":
		r.handleRollback()
	case "/exit", "/quit", "/q":
		return true
	default:
		fmt.Fprintf(r.Out, "%s Unknown command: %s (type /help)\n", tui.Warning("?"), input)
	}
	return false
}

func (r *REPL) printHelp() {
	fmt.Fprintln(r.Out, tui.Header("Available commands"))
	cmds := []struct{ cmd, desc string }{
		{"/help", "show this help"},
		{"/clear", "clear screen and redraw banner"},
		{"/context", "show project context summary"},
		{"/files [dir]", "list files via the agent's file tool"},
		{"/search <query>", "search files in the workspace"},
		{"/models", "list local models"},
		{"/model", "show the currently selected model"},
		{"/runtime", "show runtime status"},
		{"/plan", "show the plan from the current/last task"},
		{"/compact", "compact conversation history"},
		{"/permissions", "show the tool permission policy"},
		{"/git", "show git diff + status"},
		{"/diff", "show git diff"},
		{"/status", "show git status"},
		{"/rollback", "revert the last APCode change set"},
		{"/exit", "exit the REPL"},
	}
	for _, c := range cmds {
		fmt.Fprintf(r.Out, "  %s %s\n", tui.Primary(fmt.Sprintf("%-9s", c.cmd)), tui.Muted(c.desc))
	}
	fmt.Fprintln(r.Out)
	fmt.Fprintln(r.Out, tui.Muted("Any other text is sent to the AI agent."))
}

func (r *REPL) handleContext() {
	if r.ProjectCtx == nil {
		fmt.Fprintln(r.Out, tui.Warning("No project context available."))
		return
	}
	fmt.Fprintf(r.Out, "%s Project: %s\n", tui.Primary("Context:"), r.Workspace)
	fmt.Fprintf(r.Out, "  Files: %d\n", len(r.ProjectCtx.Files))
	fmt.Fprintf(r.Out, "  Languages: ")
	langs := []string{}
	for l, c := range r.ProjectCtx.Languages {
		langs = append(langs, fmt.Sprintf("%s (%d)", l, c))
	}
	if len(langs) > 0 {
		fmt.Fprintln(r.Out, strings.Join(langs, ", "))
	} else {
		fmt.Fprintln(r.Out, "none")
	}
	fmt.Fprintf(r.Out, "  Tokens: %d\n", r.ProjectCtx.TotalTokens)
	if r.IsGitRepo {
		fmt.Fprintf(r.Out, "  Git: %s branch %s\n", tui.Success("yes"), r.GitBranch)
	}
}

func (r *REPL) handleModels() {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		_ = registry.Add(m)
	}
	manager, err := localmodel.NewManager(r.ModelDir, registry)
	if err != nil {
		fmt.Fprintf(r.ErrOut, "Failed to load models: %v\n", err)
		return
	}
	all := manager.ListAll()
	fmt.Fprintln(r.Out, tui.Primary("Models:"))
	for _, m := range all {
		status := tui.Warning("not installed")
		if m.Installed {
			status = tui.Success("installed")
		}
		fmt.Fprintf(r.Out, "  %s (%s) - %s\n", m.Name, m.ID, status)
	}
}

func (r *REPL) handleRuntime() {
	if r.Runtime == nil {
		fmt.Fprintln(r.Out, tui.Warning("No runtime detected"))
		return
	}
	st, _ := r.Runtime.Status(context.Background())
	fmt.Fprintf(r.Out, "%s Runtime: %s (%s)\n", tui.Primary("Runtime:"), st.Type, st.State)
	fmt.Fprintf(r.Out, "  Available: %v\n", st.Available)
	if st.ModelID != "" {
		fmt.Fprintf(r.Out, "  Model: %s\n", st.ModelID)
	}
	if r.Model != nil {
		fmt.Fprintf(r.Out, "  Local model: %s\n", r.Model.Name)
	} else {
		fmt.Fprintln(r.Out, tui.Warning("  No local model installed"))
	}
}

func (r *REPL) handleDiff(ctx context.Context) {
	tool, ok := r.Registry.Get("git_diff")
	if !ok {
		tool, ok = r.Registry.Get("GitDiff")
	}
	if !ok {
		fmt.Fprintln(r.ErrOut, "git_diff tool not available")
		return
	}
	res, err := tool.Execute(ctx, tools.Input{})
	if err != nil {
		fmt.Fprintf(r.ErrOut, "diff error: %v\n", err)
		return
	}
	if res.Err != nil {
		fmt.Fprintf(r.Out, "%s %v\n", tui.Warning("Diff:"), res.Err)
		if res.Output != "" {
			fmt.Fprintln(r.Out, res.Output)
		}
		return
	}
	if strings.TrimSpace(res.Output) == "" {
		fmt.Fprintln(r.Out, tui.Muted("No changes."))
		return
	}
	fmt.Fprintln(r.Out, tui.Primary("Diff:"))
	fmt.Fprintln(r.Out, res.Output)
}

func (r *REPL) handleStatus() {
	tool, ok := r.Registry.Get("git_status")
	if !ok {
		tool, ok = r.Registry.Get("GitStatus")
	}
	if !ok {
		fmt.Fprintln(r.ErrOut, "git_status tool not available")
		return
	}
	res, err := tool.Execute(context.Background(), tools.Input{})
	if err != nil {
		fmt.Fprintf(r.ErrOut, "status error: %v\n", err)
		return
	}
	if res.Err != nil {
		fmt.Fprintf(r.Out, "%s %v\n", tui.Warning("Status:"), res.Err)
		return
	}
	fmt.Fprintln(r.Out, tui.Primary("Status:"))
	fmt.Fprintln(r.Out, res.Output)
	if strings.TrimSpace(res.Output) == "" {
		fmt.Fprintln(r.Out, tui.Muted("Working tree clean."))
	}
}

// runAgent handles normal user text via agent/runtime with loop and history.
func (r *REPL) runAgent(ctx context.Context, prompt string) (string, error) {
	if r.Runtime == nil {
		return "", fmt.Errorf("No runtime available.\n\nRun:\n  apcode runtime\n\nInstall a runtime to enable inference.")
	}
	if r.Model == nil {
		return "No local model is installed.\n\nUse:\n  apcode models\n\nA local model is required for AI inference.", nil
	}
	if err := r.Runtime.Load(ctx, r.Model); err != nil {
		// Handle specific error states with useful messages
		var re *runtime.RuntimeError
		if errors.As(err, &re) {
			switch re.Code {
			case runtime.CodeInsufficientMemory:
				return "", fmt.Errorf("Insufficient RAM to load model %s (%s required). Try a smaller model like gemma-2b-q4 or close other applications.\nRun: apcode models", r.Model.ID, formatBytes(r.Model.MinimumRAMBytes))
			case runtime.CodeModelCorrupted:
				return "", fmt.Errorf("Model file corrupted for %s at %s. Reinstall with: apcode models install %s", r.Model.ID, r.Model.InstallPath, r.Model.ID)
			case runtime.CodeModelNotInstalled:
				return "", fmt.Errorf("Model file missing for %s at %s. Reinstall with: apcode models install %s", r.Model.ID, r.Model.InstallPath, r.Model.ID)
			case runtime.CodeIncompatibleModel:
				return "", fmt.Errorf("Model %s is incompatible with runtime %s. Try: apcode recommend", r.Model.ID, r.Runtime.Type())
			case runtime.CodeRuntimeUnavailable:
				return "", fmt.Errorf("Runtime %s unavailable. Check: apcode runtime", r.Runtime.Type())
			}
		}
		return "", fmt.Errorf("Failed to load model %s: %v", r.Model.ID, err)
	}
	defer func() { _ = r.Runtime.Unload(context.Background()) }()

	const maxIter = 10
	const maxRepairAttempts = 2
	modified := false
	repairAttempts := 0
	planEmitted := false
	// The agent system prompt: instructs the model to behave like a software
	// engineer — inspect before modifying, prefer tools, minimal changes.
	systemPrompt := "You are APCode, an offline AI coding agent working inside the user's project. " +
		"Rules: inspect before modifying; use tools instead of guessing; make minimal changes that preserve existing architecture; " +
		"avoid unrelated changes; validate changes by running tests when available; never claim success without verification. " +
		"To act, respond with JSON: {\"tool\":\"<name>\",\"input\":{...}}. To finish, respond with plain text only."
	for iter := 0; iter < maxIter; iter++ {
		historyStr := ""
		if len(r.History) > 0 {
			start := 0
			if len(r.History) > 10 {
				start = len(r.History) - 10
			}
			var b strings.Builder
			for i := start; i < len(r.History); i++ {
				m := r.History[i]
				b.WriteString(m.Role)
				b.WriteString(": ")
				b.WriteString(m.Content)
				b.WriteString("\n")
			}
			historyStr = b.String()
		}
		fullPrompt := prompt
		if iter > 0 {
			fullPrompt = historyStr
		} else if historyStr != "" {
			fullPrompt = "Conversation history:\n" + historyStr + "\nCurrent prompt: " + prompt
		}
		if r.ProjectCtx != nil && len(r.ProjectCtx.Files) > 0 && iter == 0 {
			fullPrompt = fmt.Sprintf("Project: %s (%d files)\n%s\n\nUser: %s", r.Workspace, len(r.ProjectCtx.Files), historyStr, prompt)
		}
		fullPrompt = systemPrompt + "\n\n" + fullPrompt
		if iter == 0 {
			fmt.Fprintln(r.Out, tui.Muted("  ⋯ thinking..."))
		}
		req := runtime.GenerateRequest{Prompt: fullPrompt, Options: runtime.GenerateOptions{MaxTokens: 512}}
		resp, err := r.Runtime.Generate(ctx, req)
		if err != nil {
			return "", fmt.Errorf("Inference failed: %v", err)
		}
		if resp == nil {
			return "", fmt.Errorf("No response from model")
		}
		text := strings.TrimSpace(resp.Text)
		if text == "" {
			text = "(no response)"
		}
		if plan := extractPlan(text); plan != "" {
			r.lastPlan = plan
			fmt.Fprintf(r.Out, "%s\n%s\n", tui.Primary("Plan:"), r.lastPlan)
		}
		r.History = append(r.History, Message{Role: "assistant", Content: text})
		toolCalls, answer, _ := parseToolCalls(text)
		if len(toolCalls) == 0 {
			// A plan without tool calls is an intermediate step: acknowledge
			// it and let the agent proceed to act. Only the first plan is
			// echoed; later plan-like answers are treated as final so that
			// genuinely textual replies still terminate the loop.
			if plan := extractPlan(text); plan != "" && !planEmitted && !modified {
				planEmitted = true
				r.History = append(r.History, Message{
					Role:    "system",
					Content: "Plan acknowledged. Proceed step by step using tools.",
				})
				continue
			}
			if answer != "" {
				text = answer
			}
			// Validation loop: if files were changed in this task, run the
			// detected test command once before accepting success. On
			// failure, feed the failure back and let the model repair,
			// bounded by maxRepairAttempts.
			if modified && repairAttempts < maxRepairAttempts {
				repairAttempts++
				fmt.Fprintln(r.Out, tui.Muted("  Running validation..."))
				vres, verr := r.Registry.Execute(ctx, "run_tests", tools.Input{})
				if tools.IsToolError(verrOr(vres.Err, verr), tools.CodeNotFound) {
					fmt.Fprintln(r.Out, tui.Muted("  No test command detected for this project; skipping validation."))
					return text, nil
				}
				var vout string
				failed := false
				if verr != nil || (vres.Err != nil) {
					failed = true
					vout = fmt.Sprintf("validation error: %v", verrOr(vres.Err, verr))
				} else {
					vout = vres.Output
					failed = vres.Err != nil
				}
				if failed {
					fmt.Fprintf(r.Out, "%s\n", tui.Error("✗ Validation failed"))
					fmt.Fprintf(r.Out, "%s\n", tui.Muted("  Investigating failure..."))
					r.History = append(r.History, Message{
						Role:    "tool_result",
						Content: "Validation (run_tests) FAILED. Output:\n" + truncateForHistory(vout, 2000) + "\nInvestigate the failure, fix the code with tools, then run run_tests again.",
					})
					continue
				}
				fmt.Fprintf(r.Out, "%s\n", tui.Success("✓ Tests passed"))
			}
			return text, nil
		}
		for _, tc := range toolCalls {
			fmt.Fprintf(r.Out, "%s %s %s\n", tui.Muted("  Tool:"), tc.Name, fmt.Sprintf("%v", tc.Input))
			// Unified approval policy: reads are automatic; file writes,
			// deletes, patches, and non-safe terminal commands require an
			// explicit [y/N] confirmation.
			if requiresUserApproval(tc.Name, tc.Input) {
				n := normalizeToolName(tc.Name)
				switch {
				case n == "runcommand" || n == "shell":
					fullCmd := strings.TrimSpace(tc.Input["command"] + " " + tc.Input["args"])
					class := tools.ClassifyCommand(fullCmd)
					label := "wants to run"
					if class == tools.ClassBlocked {
						label = "BLOCKED command (refused by security policy)"
					}
					fmt.Fprintf(r.Out, "\n%s %s:\n\n%s\n\n", tui.Warning("APCode"), label, fullCmd)
					fmt.Fprint(r.Out, "Allow? [y/N] ")
				case n == "deletefile":
					path := tc.Input["path"]
					fmt.Fprintf(r.Out, "\n%s wants to DELETE:\n\n%s\n\n", tui.Warning("APCode"), path)
					fmt.Fprint(r.Out, "Approve deletion? [y/N] ")
				default:
					path := tc.Input["path"]
					content := tc.Input["content"]
					patch := tc.Input["patch"]
					fmt.Fprintf(r.Out, "\n%s wants to modify:\n\n%s\n\n", tui.Primary("APCode"), tui.Warning(path))
					preview := content
					if preview == "" {
						preview = patch
					}
					if len(preview) > 500 {
						preview = preview[:500] + "..."
					}
					if preview != "" {
						fmt.Fprintln(r.Out, preview)
					}
					fmt.Fprint(r.Out, "\nApply this change? [y/N] ")
				}
				if r.reader == nil {
					r.reader = bufio.NewReader(r.In)
				}
				confirm, _ := r.reader.ReadString('\n')
				confirm = strings.ToLower(strings.TrimSpace(confirm))
				if confirm != "y" && confirm != "yes" {
					fmt.Fprintln(r.Out, tui.Muted("Skipped."))
					r.History = append(r.History, Message{Role: "tool_result", Content: "Tool " + tc.Name + " cancelled by user"})
					continue
				}
				if normalizeToolName(tc.Name) == "runcommand" || normalizeToolName(tc.Name) == "shell" {
					tc.Input["confirm"] = "true"
				}
			}
			fmt.Fprintf(r.Out, "%s %s...\n", tui.Muted("  Executing:"), tc.Name)
			tool, ok := r.Registry.Get(tc.Name)
			if !ok {
				r.History = append(r.History, Message{Role: "tool_result", Content: fmt.Sprintf("Tool %s not found", tc.Name)})
				continue
			}
			res, err := tool.Execute(ctx, tc.Input)
			var out string
			if err != nil {
				out = "Tool error: " + err.Error()
			} else if res.Err != nil {
				out = "Tool failed: " + res.Err.Error() + "\nOutput: " + res.Output
			} else {
				out = res.Output
				if out == "" {
					out = "(no output)"
				}
				if isModifyingTool(tc.Name) {
					modified = true
				}
			}
			preview := out
			if len(preview) > 200 {
				preview = preview[:200]
			}
			fmt.Fprintf(r.Out, "%s\n", tui.Muted("  Result: "+preview))
			r.History = append(r.History, Message{Role: "tool_result", Content: "Tool " + tc.Name + " result: " + out})
		}
		if iter == maxIter-1 {
			return "Max iterations reached without final answer", nil
		}
	}
	return "(no response)", nil
}

// extractPlan captures a numbered plan (3+ numbered steps or a "Plan:"
// heading followed by numbered steps) from assistant output.
func extractPlan(text string) string {
	lines := strings.Split(text, "\n")
	var planLines []string
	inPlan := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		lower := strings.ToLower(t)
		if strings.HasPrefix(lower, "plan:") || strings.HasPrefix(lower, "plan :") {
			inPlan = true
			planLines = append(planLines[:0], l)
			continue
		}
		isNumbered := len(t) > 2 && (t[0] >= '1' && t[0] <= '9') && (t[1] == '.' || t[1] == ')')
		if isNumbered {
			if !inPlan {
				inPlan = true
				planLines = nil
			}
			planLines = append(planLines, l)
		} else if inPlan {
			break
		}
	}
	if len(planLines) >= 3 {
		return strings.TrimRight(strings.Join(planLines, "\n"), "\n")
	}
	return ""
}

// verrOr returns whichever error is non-nil.
func verrOr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// truncateForHistory limits a string for inclusion in conversation history.
func truncateForHistory(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}

func parseToolCalls(text string) ([]struct {
	Name  string
	Input map[string]string
}, string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, "", nil
	}
	// Try to extract JSON block and use proper JSON unmarshaling to handle escapes
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start != -1 && end != -1 && end > start && strings.Contains(trimmed, "\"tool\"") {
		sub := trimmed[start : end+1]
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(sub), &obj); err == nil {
			// Check for tool field
			toolName := ""
			if v, ok := obj["tool"]; ok {
				if s, ok := v.(string); ok {
					toolName = s
				}
			}
			if toolName != "" {
				input := make(map[string]string)
				if raw, ok := obj["input"]; ok {
					if m, ok := raw.(map[string]interface{}); ok {
						for k, v := range m {
							input[k] = fmt.Sprint(v)
						}
					}
				}
				return []struct {
					Name  string
					Input map[string]string
				}{{Name: toolName, Input: input}}, "", nil
			}
		}
		// Try array form
		var arr []map[string]interface{}
		if err := json.Unmarshal([]byte(sub), &arr); err == nil && len(arr) > 0 {
			var calls []struct {
				Name  string
				Input map[string]string
			}
			for _, m := range arr {
				if tn, ok := m["tool"].(string); ok && tn != "" {
					input := make(map[string]string)
					if raw, ok := m["input"]; ok {
						if mm, ok := raw.(map[string]interface{}); ok {
							for k, v := range mm {
								input[k] = fmt.Sprint(v)
							}
						}
					}
					calls = append(calls, struct {
						Name  string
						Input map[string]string
					}{Name: tn, Input: input})
				}
			}
			if len(calls) > 0 {
				return calls, "", nil
			}
		} else if strings.HasPrefix(sub, "{") && strings.HasSuffix(sub, "}") && strings.Count(sub, "\"tool\"") > 1 {
			// Multiple concatenated JSON objects like {...},{...} - wrap in brackets
			var arr2 []map[string]interface{}
			if err := json.Unmarshal([]byte("["+sub+"]"), &arr2); err == nil && len(arr2) > 0 {
				var calls []struct {
					Name  string
					Input map[string]string
				}
				for _, m := range arr2 {
					if tn, ok := m["tool"].(string); ok && tn != "" {
						input := make(map[string]string)
						if raw, ok := m["input"]; ok {
							if mm, ok := raw.(map[string]interface{}); ok {
								for k, v := range mm {
									input[k] = fmt.Sprint(v)
								}
							}
						}
						calls = append(calls, struct {
							Name  string
							Input map[string]string
						}{Name: tn, Input: input})
					}
				}
				if len(calls) > 0 {
					return calls, "", nil
				}
			}
		}
	}
	return nil, trimmed, nil
}

func formatBytes(bytes uint64) string {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
	)
	switch {
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

func init() {
	_ = config.Version
}

// ---- agent-facing slash commands ----

// handleFiles lists files using the registry's file listing tool.
func (r *REPL) handleFiles() {
	dir := "."
	tool, ok := r.Registry.Get("list_files")
	if !ok {
		tool, ok = r.Registry.Get("ListDirectory")
	}
	if !ok {
		fmt.Fprintln(r.ErrOut, "file listing tool not available")
		return
	}
	res, err := tool.Execute(context.Background(), tools.Input{"path": dir})
	if err != nil || res.Err != nil {
		fmt.Fprintf(r.ErrOut, "list failed: %v\n", verrOr(res.Err, err))
		return
	}
	fmt.Fprintln(r.Out, tui.Header("Files"))
	fmt.Fprintln(r.Out, res.Output)
}

// handleSearch runs a workspace search through the registry.
func (r *REPL) handleSearch(ctx context.Context, input string) {
	query := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/search"))
	if query == "" {
		fmt.Fprintln(r.Out, tui.Muted("usage: /search <query>"))
		return
	}
	tool, ok := r.Registry.Get("search")
	if !ok {
		tool, ok = r.Registry.Get("SearchFiles")
	}
	if !ok {
		fmt.Fprintln(r.ErrOut, "search tool not available")
		return
	}
	res, err := tool.Execute(ctx, tools.Input{"query": query})
	if err != nil {
		fmt.Fprintf(r.ErrOut, "search error: %v\n", err)
		return
	}
	if res.Err != nil {
		fmt.Fprintf(r.Out, "%s %v\n", tui.Warning("Search:"), res.Err)
		return
	}
	out := res.Output
	if strings.TrimSpace(out) == "" {
		out = "(no results)"
	}
	fmt.Fprintf(r.Out, "%s %s\n", tui.Header("Search"), tui.Muted(query))
	fmt.Fprintln(r.Out, out)
}

// handleModel shows the currently selected model.
func (r *REPL) handleModel() {
	if r.Model == nil {
		fmt.Fprintln(r.Out, tui.Warning("No local model selected."))
		fmt.Fprintln(r.Out, tui.Muted("Use /models to list and `apcode models install <id>` to install."))
		return
	}
	rt := r.RuntimeName
	if rt == "" && r.Runtime != nil {
		rt = string(r.Runtime.Type())
	}
	fmt.Fprintln(r.Out, tui.Box("Model", []string{
		fmt.Sprintf("%s %s", tui.Muted("ID      :"), r.Model.ID),
		fmt.Sprintf("%s %s", tui.Muted("Name    :"), r.Model.Name),
		fmt.Sprintf("%s %s", tui.Muted("Runtime :"), rt),
	}))
}

// handlePlan shows the plan captured from the last task.
func (r *REPL) handlePlan() {
	if r.lastPlan == "" {
		fmt.Fprintln(r.Out, tui.Muted("No plan recorded yet. Plans appear when the model proposes numbered steps for a task."))
		return
	}
	fmt.Fprintln(r.Out, tui.Header("Last Plan"))
	fmt.Fprintln(r.Out, r.lastPlan)
}

// handleCompact compacts conversation history, keeping the first user
// message (the task anchor) and the most recent turns.
func (r *REPL) handleCompact() {
	const keepRecent = 6
	if len(r.History) <= keepRecent+1 {
		fmt.Fprintln(r.Out, tui.Muted(fmt.Sprintf("History is already compact (%d messages).", len(r.History))))
		return
	}
	dropped := len(r.History) - keepRecent - 1
	compacted := []Message{r.History[0]}
	compacted = append(compacted, Message{
		Role:    "system",
		Content: fmt.Sprintf("[history compacted: %d earlier messages summarized away]", dropped),
	})
	compacted = append(compacted, r.History[len(r.History)-keepRecent:]...)
	r.History = compacted
	fmt.Fprintf(r.Out, "%s\n", tui.Success(fmt.Sprintf("✓ Compacted: removed %d older messages.", dropped)))
}

// handlePermissions prints the current permission policy.
func (r *REPL) handlePermissions() {
	fmt.Fprintln(r.Out, tui.Box("Permissions", []string{
		fmt.Sprintf("%s %s", tui.Muted("Read tools        :"), tui.Success("automatic")),
		fmt.Sprintf("%s %s", tui.Muted("File writes/edits :"), tui.Warning("approval required")),
		fmt.Sprintf("%s %s", tui.Muted("File deletion     :"), tui.Warning("approval required (always)")),
		fmt.Sprintf("%s %s", tui.Muted("Safe commands     :"), tui.Success("automatic (go test, git status, ...)")),
		fmt.Sprintf("%s %s", tui.Muted("Other commands    :"), tui.Warning("approval required")),
		fmt.Sprintf("%s %s", tui.Muted("Blocked commands  :"), tui.Error("never executed")),
		fmt.Sprintf("%s %s", tui.Muted("Git commit/push   :"), tui.Error("never automatic")),
	}))
}

// handleRollback reverts the last APCode change set via the journal.
func (r *REPL) handleRollback() {
	if r.Journal == nil || r.Journal.UndoCount() == 0 {
		fmt.Fprintln(r.Out, tui.Muted("Nothing to roll back — no APCode changes recorded in this session."))
		return
	}
	fmt.Fprintln(r.Out, tui.Muted("Reverting the last APCode change..."))
	restored, err := r.Journal.Undo()
	if err != nil {
		fmt.Fprintf(r.Out, "%s %v\n", tui.Error("✗ Rollback failed:"), err)
		return
	}
	if len(restored) == 0 {
		fmt.Fprintln(r.Out, tui.Muted("No files were changed in that operation."))
		return
	}
	fmt.Fprintf(r.Out, "%s Restored %d file(s):\n", tui.Success("✓"), len(restored))
	for _, p := range restored {
		fmt.Fprintf(r.Out, "  %s\n", tui.Muted(p))
	}
}
