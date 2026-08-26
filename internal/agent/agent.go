// Package agent implements APCode's coding agent loop: the cycle of
// understanding a task, planning, invoking tools, and verifying results.
//
// Architecture implemented:
//
//	User -> Agent -> Context -> Local Model -> Tool call -> Tool result -> Model -> Verification -> Final response
//
// Loop guarantees:
//   - User prompt is validated and preserved.
//   - Project context is gathered via Provider with cancellation and error recovery.
//   - Local model is invoked via InferenceRuntime (Generate or Stream).
//   - Tool selection parses model output and validates against the tool registry.
//   - Tool execution delegates to tools.Tool (no direct filesystem access from agent).
//   - Tool result handling feeds results back into the next model prompt.
//   - Multi-step reasoning loop is bounded by MaxIterations.
//   - Context cancellation is respected at every step (context gathering, model invoke, tool exec, iteration).
//   - Maximum iteration limit prevents infinite loops.
//   - Error recovery handles transient model/tool/verification failures without crashing.
//   - Streaming is supported when configured.
//
// The agent MUST NOT directly manipulate files; all filesystem/terminal operations
// go through tools.Tool implementations.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	projectcontext "apcode/internal/context"
	"apcode/internal/runtime"
	"apcode/internal/tools"
	"apcode/internal/verification"
)

// Sentinel errors.
var (
	ErrNotImplemented     = errors.New("agent: not implemented")
	ErrEmptyInstruction   = errors.New("agent: instruction cannot be empty")
	ErrMaxIterations      = errors.New("agent: maximum iterations reached")
	ErrNoRuntime          = errors.New("agent: no runtime configured")
	ErrVerificationFailed = errors.New("agent: verification failed")
)

// Default configuration constants.
const (
	DefaultMaxIterations = 10
	MaxMaxIterations     = 50
	DefaultSystemPrompt  = "You are APCode, an offline-first AI coding agent. Use tools to help the user."
)

// Task is a unit of work requested by the user.
type Task struct {
	Instruction string
}

// Config controls agent loop behaviour.
type Config struct {
	// MaxIterations bounds the loop to prevent infinite execution. 0 means DefaultMaxIterations. Capped at MaxMaxIterations.
	MaxIterations int
	// EnableStreaming when true uses runtime.Stream; otherwise runtime.Generate.
	EnableStreaming bool
	// SystemPrompt prepended to every model prompt.
	SystemPrompt string
	// VerificationDir is the directory passed to Verifier.Verify. Empty means ".".
	VerificationDir string
	// StreamingCallback, if non-nil, is invoked for each streaming chunk token.
	StreamingCallback func(token string)
}

// MessageRole identifies a message in the loop history.
type MessageRole string

const (
	RoleUser       MessageRole = "user"
	RoleAssistant  MessageRole = "assistant"
	RoleSystem     MessageRole = "system"
	RoleToolCall   MessageRole = "tool_call"
	RoleToolResult MessageRole = "tool_result"
)

// Message records one step of the conversation/history.
type Message struct {
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
}

// ToolCall represents a parsed tool invocation requested by the model.
type ToolCall struct {
	ID    string      `json:"id,omitempty"`
	Name  string      `json:"name"`
	Input tools.Input `json:"input"`
}

// Result is the final outcome of an agent run.
type Result struct {
	// Response is the final answer text from the model (trimmed).
	Response string
	// Iterations is the number of model invocations performed.
	Iterations int
	// ToolCalls is the number of tool calls executed.
	ToolCalls int
	// Finished indicates whether the loop ended with a final answer (true) or was terminated (false, e.g., max iterations).
	Finished bool
	// History is the full loop history for inspection.
	History []Message
	// Verification holds the verifier report when verification was run.
	Verification *verification.Report
	// ContextUsed is the project context that was gathered (may be empty).
	ContextUsed string
}

// Agent executes coding tasks using context, local model, tools, and verification.
type Agent struct {
	rt       runtime.InferenceRuntime
	provider projectcontext.Provider
	registry *tools.Registry
	verifier verification.Verifier
	cfg      Config
}

// AgentInterface defines the agent execution contract.
type AgentInterface interface {
	Run(ctx context.Context, t Task) error
	RunWithResult(ctx context.Context, t Task) (*Result, error)
}

// Ensure Agent implements AgentInterface.
var _ AgentInterface = (*Agent)(nil)

// New creates a new Agent. Any dependency may be nil (handled gracefully):
//   - nil runtime: Run will return ErrNoRuntime unless GenerateFunc is mocked via runtime.
//   - nil provider: context gathering is skipped.
//   - nil registry: tool execution will report "tool not found" but still loop.
//   - nil verifier: verification is skipped.
func New(rt runtime.InferenceRuntime, provider projectcontext.Provider, registry *tools.Registry, verifier verification.Verifier, cfg Config) *Agent {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = DefaultMaxIterations
	}
	if cfg.MaxIterations > MaxMaxIterations {
		cfg.MaxIterations = MaxMaxIterations
	}
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		cfg.SystemPrompt = DefaultSystemPrompt
	}
	if cfg.VerificationDir == "" {
		cfg.VerificationDir = "."
	}
	if registry == nil {
		registry = tools.NewRegistry()
	}
	return &Agent{
		rt:       rt,
		provider: provider,
		registry: registry,
		verifier: verifier,
		cfg:      cfg,
	}
}

// Run implements the original Agent interface for backward compatibility.
// It delegates to RunWithResult and returns only the error; the result is discarded.
// Prefer RunWithResult when you need the final response.
func (a *Agent) Run(ctx context.Context, t Task) error {
	_, err := a.RunWithResult(ctx, t)
	return err
}

// RunWithResult executes the full agent loop and returns a Result.
// It respects ctx cancellation, enforces MaxIterations, recovers from transient errors,
// supports streaming, and never directly manipulates files.
func (a *Agent) RunWithResult(ctx context.Context, t Task) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(t.Instruction) == "" {
		return nil, ErrEmptyInstruction
	}
	if a.rt == nil {
		return nil, ErrNoRuntime
	}

	// 1. Context gathering (with cancellation and error recovery).
	var contextData []byte
	var contextStr string
	if a.provider != nil {
		// Use GatherWithContext when available to respect cancellation.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		data, err := a.provider.GatherWithContext(ctx, t.Instruction)
		if err != nil {
			// Error recovery: do not fail the whole run; log as history and continue with empty context.
			// Cancellation errors are still propagated.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "cancel") {
				return nil, err
			}
			contextData = []byte{}
			contextStr = ""
			// We will record this in history below; continue.
		} else {
			contextData = data
			contextStr = string(data)
		}
	}

	history := []Message{
		{Role: RoleUser, Content: t.Instruction},
	}
	if contextStr != "" {
		history = append(history, Message{Role: RoleSystem, Content: "Project context:\n" + truncateContext(contextStr, 8000)})
	}
	if len(contextData) == 0 && a.provider != nil {
		// Record that context gathering produced no data or error was recovered.
		// We add a system note only if there was an error? For observability we keep it minimal.
	}

	result := &Result{
		History:     history,
		ContextUsed: contextStr,
	}

	var totalToolCalls int

	// Multi-step reasoning loop (bounded).
	for iter := 0; iter < a.cfg.MaxIterations; iter++ {
		select {
		case <-ctx.Done():
			result.Iterations = iter
			result.ToolCalls = totalToolCalls
			result.History = history
			return result, ctx.Err()
		default:
		}

		// 3. Model invocation (with streaming support and error recovery).
		prompt := a.buildPrompt(t, contextStr, history)

		var modelText string
		var modelErr error
		if a.cfg.EnableStreaming {
			modelText, modelErr = a.invokeStream(ctx, prompt)
		} else {
			modelText, modelErr = a.invokeGenerate(ctx, prompt)
		}

		if modelErr != nil {
			// Cancellation is not recoverable.
			if isCancelledError(modelErr) || errors.Is(modelErr, context.Canceled) || errors.Is(modelErr, context.DeadlineExceeded) {
				result.Iterations = iter + 1
				result.ToolCalls = totalToolCalls
				result.History = history
				return result, modelErr
			}
			// Error recovery: record the model error and retry next iteration if budget remains.
			// If this was the last iteration, return the error.
			history = append(history, Message{
				Role:    RoleSystem,
				Content: fmt.Sprintf("model invocation failed (iteration %d/%d): %v — recovering and retrying", iter+1, a.cfg.MaxIterations, modelErr),
			})
			result.History = history
			result.Iterations = iter + 1
			if iter == a.cfg.MaxIterations-1 {
				return result, fmt.Errorf("agent: model invocation failed after %d iterations: %w", iter+1, modelErr)
			}
			// Brief backoff respecting cancellation (max 50ms) to avoid tight retry loop.
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}

		// Record assistant raw output.
		assistantMsg := Message{Role: RoleAssistant, Content: modelText}

		// 4. Tool selection: parse tool calls from model output.
		toolCalls, answer, _ := parseModelOutput(modelText)
		assistantMsg.ToolCalls = toolCalls
		history = append(history, assistantMsg)

		if len(toolCalls) == 0 {
			// No tool calls -> final answer.
			finalAnswer := strings.TrimSpace(answer)
			if finalAnswer == "" {
				finalAnswer = strings.TrimSpace(modelText)
			}
			result.Response = finalAnswer
			result.Iterations = iter + 1
			result.ToolCalls = totalToolCalls
			result.Finished = true
			result.History = history

			// Verification step (with error recovery).
			if a.verifier != nil {
				vCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				report, vErr := a.verifier.Verify(vCtx, a.cfg.VerificationDir)
				cancel()
				if vErr != nil {
					if isCancelledError(vErr) || errors.Is(vErr, context.Canceled) {
						return result, vErr
					}
					// Recovery: attach failed verification but do not abort final response.
					result.Verification = &verification.Report{Passed: false, Output: fmt.Sprintf("verification error: %v", vErr)}
				} else {
					result.Verification = &report
					if !report.Passed {
						// Mark but still return response; caller can inspect.
					}
				}
			}
			return result, nil
		}

		// 5. Tool execution (via tools, never direct file manipulation).
		// 6. Tool result handling: feed back into history.
		for _, tc := range toolCalls {
			select {
			case <-ctx.Done():
				result.Iterations = iter + 1
				result.ToolCalls = totalToolCalls
				result.History = history
				return result, ctx.Err()
			default:
			}
			totalToolCalls++

			tool, ok := a.registry.Get(tc.Name)
			if !ok {
				// Error recovery: the model hallucinated a tool name.
				// Return the canonical structured error including the real
				// tool list so the model can pick an actual tool next turn.
				history = append(history, Message{
					Role:    RoleToolResult,
					Content: a.registry.UnknownToolPayload(tc.Name) + "\nRecover by selecting a real tool from available_tools and retrying.",
				})
				continue
			}
			if verr := a.registry.Validate(tc.Name, tc.Input); verr != nil {
				history = append(history, Message{
					Role:    RoleToolResult,
					Content: "invalid_tool_call: " + verr.Error(),
				})
				continue
			}

			// Execute with context cancellation respected.
			// Tool implementations themselves handle their own cancellation; we just pass ctx.
			execResult, execErr := tool.Execute(ctx, tc.Input)
			var out string
			if execErr != nil {
				if isCancelledError(execErr) || errors.Is(execErr, context.Canceled) {
					result.Iterations = iter + 1
					result.ToolCalls = totalToolCalls
					result.History = history
					return result, execErr
				}
				// Recovery: include execution error as tool result and continue.
				out = fmt.Sprintf("tool %q execution transport error: %v", tc.Name, execErr)
				if execResult.Output != "" {
					out += "\noutput: " + execResult.Output
				}
			} else {
				out = execResult.Output
				if execResult.Err != nil {
					// Tool reported application error; keep output but also surface error string for model.
					if out != "" {
						out = fmt.Sprintf("error: %v\noutput: %s", execResult.Err, out)
					} else {
						out = fmt.Sprintf("error: %v", execResult.Err)
					}
				}
				if out == "" {
					out = "(no output)"
				}
			}

			// Record tool result.
			history = append(history, Message{
				Role:    RoleToolCall,
				Content: fmt.Sprintf("called %s with %v", tc.Name, tc.Input),
			})
			history = append(history, Message{
				Role:    RoleToolResult,
				Content: fmt.Sprintf("tool %s result: %s", tc.Name, truncateContext(out, 4000)),
			})

			// Optional: check cancellation again after each tool.
			select {
			case <-ctx.Done():
				result.Iterations = iter + 1
				result.ToolCalls = totalToolCalls
				result.History = history
				return result, ctx.Err()
			default:
			}
		}

		// Loop continues to next iteration where history (including tool results) is included in prompt.
		result.History = history
	}

	// 9. Maximum iteration limit reached -> do not loop infinitely.
	result.Iterations = a.cfg.MaxIterations
	result.ToolCalls = totalToolCalls
	result.Finished = false
	result.History = history
	return result, fmt.Errorf("%w: %d iterations without final answer", ErrMaxIterations, a.cfg.MaxIterations)
}

// buildPrompt assembles the prompt for the model from system prompt, user instruction, context, and history.
func (a *Agent) buildPrompt(t Task, contextStr string, history []Message) string {
	var b strings.Builder
	b.WriteString(a.cfg.SystemPrompt)
	b.WriteString("\n\n")
	if contextStr != "" {
		b.WriteString("Project context:\n")
		b.WriteString(truncateContext(contextStr, 6000))
		b.WriteString("\n\n")
	}
	// Tool definitions so model knows how to call tools.
	b.WriteString(a.registry.DefinitionsForPrompt())
	b.WriteString("\n")
	// History: include prior tool results and assistant messages.
	// We keep last 20 messages to bound prompt size.
	start := 0
	if len(history) > 20 {
		start = len(history) - 20
	}
	for i := start; i < len(history); i++ {
		m := history[i]
		switch m.Role {
		case RoleUser:
			fmt.Fprintf(&b, "User: %s\n", m.Content)
		case RoleAssistant:
			fmt.Fprintf(&b, "Assistant: %s\n", m.Content)
		case RoleSystem:
			fmt.Fprintf(&b, "System: %s\n", m.Content)
		case RoleToolCall:
			fmt.Fprintf(&b, "ToolCall: %s\n", m.Content)
		case RoleToolResult:
			fmt.Fprintf(&b, "ToolResult: %s\n", m.Content)
		default:
			fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
		}
	}
	b.WriteString("\nUser request: ")
	b.WriteString(t.Instruction)
	b.WriteString("\nRespond with either a tool call JSON or a final answer.\n")
	return b.String()
}

// invokeGenerate calls the local model non-streaming.
func (a *Agent) invokeGenerate(ctx context.Context, prompt string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	req := runtime.GenerateRequest{
		Prompt: prompt,
	}
	resp, err := a.rt.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("agent: nil generate response")
	}
	return resp.Text, nil
}

// invokeStream calls the local model streaming and concatenates tokens.
// It respects ctx cancellation and invokes the streaming callback if configured.
func (a *Agent) invokeStream(ctx context.Context, prompt string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	req := runtime.GenerateRequest{
		Prompt: prompt,
	}
	ch, err := a.rt.Stream(ctx, req)
	if err != nil {
		return "", err
	}
	if ch == nil {
		return "", errors.New("agent: nil stream channel")
	}
	var b strings.Builder
	for chunk := range ch {
		select {
		case <-ctx.Done():
			return b.String(), ctx.Err()
		default:
		}
		if chunk.Error != nil {
			// If stream reported cancellation, propagate.
			if isCancelledError(chunk.Error) {
				return b.String(), chunk.Error
			}
			return b.String(), chunk.Error
		}
		if chunk.Token != "" {
			b.WriteString(chunk.Token)
			if a.cfg.StreamingCallback != nil {
				a.cfg.StreamingCallback(chunk.Token)
			}
		}
		if chunk.Done {
			if chunk.FinishReason == "cancelled" && chunk.Error != nil {
				return b.String(), chunk.Error
			}
			// If cancelled without error but finish is cancelled, treat as cancelled.
			if chunk.FinishReason == "cancelled" {
				return b.String(), context.Canceled
			}
			break
		}
	}
	return b.String(), nil
}

// parseModelOutput attempts to extract tool calls from model text.
// Supports:
//   - Single JSON object: {"tool":"name","input":{...}}
//   - Array of tool calls: [{"tool":"...", "input":{...}}]
//   - Wrapper: {"tool_calls": [...]}
//   - JSON inside ```json fences
//   - TOOL: <name> <k=v>... lines
//   - Plain text is treated as final answer.
func parseModelOutput(text string) ([]ToolCall, string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, "", nil
	}

	// Try JSON inside fences first.
	if strings.Contains(trimmed, "```") {
		if calls, ok := tryParseJSON(extractFencedJSON(trimmed)); ok && len(calls) > 0 {
			return calls, "", nil
		}
		// Also try the whole trimmed as fallback after extraction attempt.
	}

	// Direct JSON object/array at start (after trimming leading whitespace).
	// Find first '{' or '[' and last '}' or ']' and try parsing that substring to be robust against surrounding text.
	// Simple approach: if trimmed starts with { or [, try full trimmed.
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if calls, ok := tryParseJSON(trimmed); ok && len(calls) > 0 {
			return calls, "", nil
		}
		// Also try to locate JSON block within text (e.g., assistant added preamble + JSON)
		if start := strings.Index(trimmed, "{"); start != -1 {
			if end := strings.LastIndex(trimmed, "}"); end != -1 && end > start {
				sub := trimmed[start : end+1]
				if calls, ok := tryParseJSON(sub); ok && len(calls) > 0 {
					// Remaining text before JSON is answer prefix; we treat tool call as primary.
					return calls, "", nil
				}
			}
		}
	}

	// TOOL: marker syntax
	if strings.Contains(trimmed, "TOOL:") {
		calls, remaining := parseToolMarker(trimmed)
		if len(calls) > 0 {
			return calls, remaining, nil
		}
	}

	// Heuristic: if text contains "\"tool\"" substring, try to parse as JSON even with surrounding prose.
	if strings.Contains(trimmed, "\"tool\"") {
		// Attempt to extract JSON object boundaries.
		start := strings.Index(trimmed, "{")
		end := strings.LastIndex(trimmed, "}")
		if start != -1 && end != -1 && end > start {
			sub := trimmed[start : end+1]
			if calls, ok := tryParseJSON(sub); ok && len(calls) > 0 {
				return calls, "", nil
			}
		}
	}

	// No tool found -> treat entire text as answer.
	return nil, trimmed, nil
}

func extractFencedJSON(s string) string {
	// Find first ``` and last ``` and extract inner content.
	first := strings.Index(s, "```")
	if first == -1 {
		return s
	}
	last := strings.LastIndex(s, "```")
	if last <= first {
		return s
	}
	inner := s[first+3 : last]
	inner = strings.TrimSpace(inner)
	// Strip optional language tag like "json"
	if strings.HasPrefix(strings.ToLower(inner), "json") {
		inner = strings.TrimSpace(inner[4:])
	}
	return inner
}

func tryParseJSON(s string) ([]ToolCall, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	// Try single object with tool/name field
	var obj map[string]any
	if err := json.Unmarshal([]byte(s), &obj); err == nil {
		if calls := extractToolCallsFromMap(obj); len(calls) > 0 {
			return calls, true
		}
	}
	// Try array of objects
	var arr []map[string]any
	if err := json.Unmarshal([]byte(s), &arr); err == nil && len(arr) > 0 {
		var all []ToolCall
		for _, m := range arr {
			if c := extractToolCallsFromMap(m); len(c) > 0 {
				all = append(all, c...)
			} else {
				// Direct object in array with tool field
				if tn := extractToolName(m); tn != "" {
					inp := extractInput(m)
					all = append(all, ToolCall{Name: tn, Input: inp})
				}
			}
		}
		if len(all) > 0 {
			return all, true
		}
	}
	return nil, false
}

func extractToolCallsFromMap(m map[string]any) []ToolCall {
	var calls []ToolCall
	// Single tool call in object
	if tn := extractToolName(m); tn != "" {
		inp := extractInput(m)
		calls = append(calls, ToolCall{Name: tn, Input: inp})
		// Note: if this object also contains tool_calls, we add those too
	}
	// tool_calls array
	if raw, ok := m["tool_calls"]; ok {
		if arr, ok := raw.([]any); ok {
			for _, item := range arr {
				if mm, ok := item.(map[string]any); ok {
					if tn := extractToolName(mm); tn != "" {
						inp := extractInput(mm)
						// Support nested "function" or "tool" structures from some models.
						calls = append(calls, ToolCall{Name: tn, Input: inp})
					}
				}
			}
		}
	}
	// Also support "tools" key alternative
	if raw, ok := m["tools"]; ok && len(calls) == 0 {
		if arr, ok := raw.([]any); ok {
			for _, item := range arr {
				if mm, ok := item.(map[string]any); ok {
					if tn := extractToolName(mm); tn != "" {
						inp := extractInput(mm)
						calls = append(calls, ToolCall{Name: tn, Input: inp})
					}
				}
			}
		}
	}
	// Filter empty-name calls that may have been added incorrectly when tool_calls exists but original object had empty tool
	// Keep only valid.
	var filtered []ToolCall
	for _, c := range calls {
		if strings.TrimSpace(c.Name) != "" {
			if c.Input == nil {
				c.Input = make(tools.Input)
			}
			filtered = append(filtered, c)
		}
	}
	// If we extracted via tool_calls and also had a single tool from same map (which would be duplicate empty), the filtered logic may retain duplicate.
	// Deduplicate: if original map had tool_calls and also a top-level tool with same name, keep all but ensure uniqueness? Keep all.
	return filtered
}

func extractToolName(m map[string]any) string {
	// Prefer "tool", fall back to "name", "function"
	for _, key := range []string{"tool", "name", "function"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				// If function is an object like {"name":"..."} handle
				return strings.TrimSpace(s)
			}
			if mm, ok := v.(map[string]any); ok {
				if n, ok := mm["name"].(string); ok {
					return strings.TrimSpace(n)
				}
			}
		}
	}
	return ""
}

func extractInput(m map[string]any) tools.Input {
	inp := make(tools.Input)
	// Check "input", "args", "arguments", "parameters"
	for _, key := range []string{"input", "args", "arguments", "parameters"} {
		if raw, ok := m[key]; ok {
			switch v := raw.(type) {
			case map[string]any:
				for k, val := range v {
					inp[k] = fmt.Sprint(val)
				}
				return inp
			case string:
				// Sometimes arguments is a JSON stringified object
				var parsed map[string]any
				if err := json.Unmarshal([]byte(v), &parsed); err == nil {
					for k, val := range parsed {
						inp[k] = fmt.Sprint(val)
					}
					return inp
				}
				// fallback: single string value? treat as content?
				inp[key] = v
				return inp
			}
		}
	}
	return inp
}

func parseToolMarker(text string) ([]ToolCall, string) {
	lines := strings.Split(text, "\n")
	var calls []ToolCall
	var remainingLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "TOOL:") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "TOOL:"))
			if rest == "" {
				continue
			}
			parts := strings.Fields(rest)
			if len(parts) == 0 {
				continue
			}
			name := parts[0]
			inp := make(tools.Input)
			for _, p := range parts[1:] {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) == 2 {
					k := strings.TrimSpace(kv[0])
					v := strings.Trim(strings.TrimSpace(kv[1]), "\"'`")
					inp[k] = v
				} else if p != "" {
					// standalone arg treated as path or command?
					if _, exists := inp["path"]; !exists {
						inp["path"] = p
					} else if _, exists := inp["command"]; !exists {
						inp["command"] = p
					}
				}
			}
			calls = append(calls, ToolCall{Name: name, Input: inp})
		} else {
			remainingLines = append(remainingLines, line)
		}
	}
	remaining := strings.TrimSpace(strings.Join(remainingLines, "\n"))
	return calls, remaining
}

func isCancelledError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cancelled") || strings.Contains(msg, "canceled") || strings.Contains(msg, "context cancelled")
}

func truncateContext(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}
