package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

)

type RunShell struct {
	allowedCommands []string
}

func (t *RunShell) Name() string        { return "run_shell" }
func (t *RunShell) Description() string { return "执行 shell 命令（仅限白名单命令）" }
func (t *RunShell) InputSchema() json.RawMessage {
	return MakeSchema(map[string]any{
		"command": map[string]any{
			"type":        "string",
			"description": "要执行的 shell 命令",
		},
	})
}

func (t *RunShell) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}

	if len(t.allowedCommands) > 0 {
		allowed := false
		for _, prefix := range t.allowedCommands {
			if strings.HasPrefix(params.Command, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("command not allowed: %s (allowed: %v)", params.Command, t.allowedCommands)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", params.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w\noutput: %s", err, output)
	}
	return string(output), nil
}

func (t *RunShell) SetAllowedCommands(cmds []string) {
	t.allowedCommands = cmds
}
