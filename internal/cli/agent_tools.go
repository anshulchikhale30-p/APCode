package cli

import (
	"context"
	"path/filepath"
	"strings"

	"apcode/internal/tools"
)

// journalingTool wraps a file-modifying tool and snapshots target files into
// a Journal before the underlying tool executes.
type journalingTool struct {
	inner tools.Tool
	ws    string
	j     *Journal
}

func (t *journalingTool) Name() string              { return t.inner.Name() }
func (t *journalingTool) Description() string       { return t.inner.Description() }
func (t *journalingTool) InputSchema() tools.Schema { return t.inner.InputSchema() }

func (t *journalingTool) Execute(ctx context.Context, in tools.Input) (tools.Result, error) {
	if p := strings.TrimSpace(in["path"]); p != "" {
		abs := filepath.Clean(filepath.Join(t.ws, filepath.FromSlash(p)))
		t.j.Record(abs)
	}
	return t.inner.Execute(ctx, in)
}

// modifyingToolNames lists tool names whose execution changes files.
var modifyingToolNames = map[string]bool{
	"writefile":  true,
	"editfile":   true,
	"createfile": true,
	"deletefile": true,
	"applypatch": true,
}

// isModifyingTool reports whether the given tool name modifies files
// (normalized lookup, underscores ignored).
func isModifyingTool(name string) bool {
	n := normalizeToolName(name)
	return modifyingToolNames[n]
}

// normalizeToolName lowercases and strips underscores/dashes.
func normalizeToolName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, "_", "")
	n = strings.ReplaceAll(n, "-", "")
	return n
}

// wrapRegistryWithJournal returns a new registry where every file-modifying
// tool from src records into j before executing. Non-modifying tools are
// carried over unchanged.
func wrapRegistryWithJournal(src *tools.Registry, ws string, j *Journal) (*tools.Registry, error) {
	dst, err := tools.NewRegistryWithWorkspace(ws)
	if err != nil {
		return nil, err
	}
	for _, t := range src.List() {
		var toRegister tools.Tool = t
		if isModifyingTool(t.Name()) {
			toRegister = &journalingTool{inner: t, ws: ws, j: j}
		}
		if err := dst.Register(toRegister); err != nil {
			// Spec aliases may collide on normalized names; keep the first.
			continue
		}
	}
	return dst, nil
}

// requiresUserApproval reports whether executing this tool call should ask
// the user first. Reads are automatic; writes/deletes/patches require
// approval; shell commands are classified separately by security class.
func requiresUserApproval(name string, in tools.Input) bool {
	if isModifyingTool(name) {
		return true
	}
	n := normalizeToolName(name)
	switch n {
	case "runcommand", "shell", "terminal":
		cmd := strings.TrimSpace(in["command"] + " " + in["args"])
		return tools.ClassifyCommand(cmd) != tools.ClassSafe
	default:
		return false
	}
}
