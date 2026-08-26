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
	NoColor     bool
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

		fmt.Fprint(r.Out, tui.Primary("You > "))

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
			if err != nil {
				if err == context.Canceled {
					fmt.Fprintln(r.Out, tui.Muted("Cancelled."))
				} else {
					fmt.Fprintf(r.Out, "%s %v\n", tui.Error("Error:"), err)
				}
				continue
			}
			r.History = append(r.History, Message{Role: "assistant", Content: response})
			fmt.Fprintf(r.Out, "\n%s %s\n\n", tui.Primary("APCode >"), response)
		}
	}
}

func (r *REPL) printBanner() {
	fmt.Fprintln(r.Out, tui.Primary(tui.Banner()))
	fmt.Fprintln(r.Out)
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
		fmt.Fprintf(r.Out, "%s %s\n", tui.Success("✓"), fmt.Sprintf("Project detected (%s, %d files)", lang, len(r.ProjectCtx.Files)))
	} else {
		fmt.Fprintf(r.Out, "%s %s\n", tui.Warning("○"), "No project detected")
	}
	if r.IsGitRepo {
		branchInfo := ""
		if r.GitBranch != "" {
			branchInfo = fmt.Sprintf(" (%s)", r.GitBranch)
		}
		fmt.Fprintf(r.Out, "%s %s\n", tui.Success("✓"), "Git repository detected"+branchInfo)
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
			fmt.Fprintf(r.Out, "%s %s\n", tui.Success("✓"), fmt.Sprintf("Runtime ready (%s)", name))
		} else {
			fmt.Fprintf(r.Out, "%s %s\n", tui.Warning("○"), "Runtime not available")
		}
	} else {
		fmt.Fprintf(r.Out, "%s %s\n", tui.Warning("○"), "No runtime detected")
	}
	if r.Model != nil {
		fmt.Fprintf(r.Out, "%s %s\n", tui.Success("✓"), fmt.Sprintf("Local model: %s (%s)", r.Model.Name, r.Model.ID))
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
				fmt.Fprintf(r.Out, "%s %s\n", tui.Success("✓"), "Model loaded")
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
	fmt.Fprintln(r.Out, tui.Muted("Type /help for commands."))
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
	case "/models", "/model":
		r.handleModels()
	case "/runtime", "/rt":
		r.handleRuntime()
	case "/diff":
		r.handleDiff(ctx)
	case "/status", "/st":
		r.handleStatus()
	case "/exit", "/quit", "/q":
		return true
	default:
		fmt.Fprintf(r.Out, "%s Unknown command: %s (type /help)\n", tui.Warning("?"), input)
	}
	return false
}

func (r *REPL) printHelp() {
	fmt.Fprintln(r.Out, tui.Primary("Available commands:"))
	fmt.Fprintln(r.Out, "  /help     - show this help")
	fmt.Fprintln(r.Out, "  /clear    - clear screen")
	fmt.Fprintln(r.Out, "  /context  - show project context")
	fmt.Fprintln(r.Out, "  /models   - list models")
	fmt.Fprintln(r.Out, "  /runtime  - show runtime status")
	fmt.Fprintln(r.Out, "  /diff     - show git diff")
	fmt.Fprintln(r.Out, "  /status   - show git status")
	fmt.Fprintln(r.Out, "  /exit     - exit REPL")
	fmt.Fprintln(r.Out, "  /quit     - exit REPL")
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
		if iter == 0 {
			fmt.Fprintln(r.Out, tui.Muted("  Thinking..."))
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
		r.History = append(r.History, Message{Role: "assistant", Content: text})
		toolCalls, answer, _ := parseToolCalls(text)
		if len(toolCalls) == 0 {
			if answer != "" {
				text = answer
			}
			return text, nil
		}
		for _, tc := range toolCalls {
			fmt.Fprintf(r.Out, "%s %s %s\n", tui.Muted("  Tool:"), tc.Name, fmt.Sprintf("%v", tc.Input))
			if tc.Name == "write_file" || tc.Name == "WriteFile" {
				path := tc.Input["path"]
				content := tc.Input["content"]
				fmt.Fprintf(r.Out, "\n%s wants to modify:\n\n%s\n\n", tui.Primary("APCode"), tui.Warning(path))
				preview := content
				if len(preview) > 500 {
					preview = preview[:500] + "..."
				}
				fmt.Fprintln(r.Out, preview)
				fmt.Fprint(r.Out, "\nApply this change? [y/N] ")
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
			} else if tc.Name == "shell" || tc.Name == "RunCommand" {
				cmd := tc.Input["command"]
				args := tc.Input["args"]
				fullCmd := cmd + " " + args
				dangerous := isDangerousCommand(fullCmd)
				if dangerous {
					fmt.Fprintf(r.Out, "\n%s wants to run dangerous command:\n\n%s\n\n", tui.Warning("APCode"), fullCmd)
					fmt.Fprint(r.Out, "Allow? [y/N] ")
					if r.reader == nil {
						r.reader = bufio.NewReader(r.In)
					}
					confirm, _ := r.reader.ReadString('\n')
					confirm = strings.ToLower(strings.TrimSpace(confirm))
					if confirm != "y" && confirm != "yes" {
						r.History = append(r.History, Message{Role: "tool_result", Content: "Command cancelled by user"})
						continue
					}
				} else {
					fmt.Fprintf(r.Out, "\n%s wants to run:\n\n%s\n\n", tui.Muted("  Running:"), fullCmd)
					fmt.Fprint(r.Out, "Run? [y/N] ")
					if r.reader == nil {
						r.reader = bufio.NewReader(r.In)
					}
					confirm, _ := r.reader.ReadString('\n')
					confirm = strings.ToLower(strings.TrimSpace(confirm))
					if confirm != "y" && confirm != "yes" {
						r.History = append(r.History, Message{Role: "tool_result", Content: "Command cancelled"})
						continue
					}
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

func isDangerousCommand(cmd string) bool {
	lower := strings.ToLower(cmd)
	dangerous := []string{"rm ", "del ", "rmdir", "format", "git reset --hard", "git clean", "mkfs"}
	for _, pat := range dangerous {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
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
