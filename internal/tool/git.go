package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

)

// ========== git_diff ==========

type GitDiff struct {
	projectRoot string
}

func (t *GitDiff) Name() string        { return "git_diff" }
func (t *GitDiff) Description() string { return "查看 Git 工作区的代码变更" }
func (t *GitDiff) InputSchema() json.RawMessage {
	return MakeSchema(map[string]any{
		"staged": map[string]any{
			"type":        "boolean",
			"description": "是否只查看已暂存的变更，默认 false",
		},
	})
}

func (t *GitDiff) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Staged bool `json:"staged"`
	}
	json.Unmarshal(input, &params)

	args := []string{"diff"}
	if params.Staged {
		args = append(args, "--staged")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = t.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git diff failed: %w", err)
	}
	if len(output) == 0 {
		return "no changes", nil
	}
	return string(output), nil
}

// ========== git_commit ==========

type GitCommit struct {
	projectRoot string
}

func (t *GitCommit) Name() string        { return "git_commit" }
func (t *GitCommit) Description() string { return "暂存并提交代码变更" }
func (t *GitCommit) InputSchema() json.RawMessage {
	return MakeSchema(map[string]any{
		"message": map[string]any{
			"type":        "string",
			"description": "提交信息",
		},
		"files": map[string]any{
			"type":        "array",
			"description": "要暂存的文件列表，为空则暂存所有变更",
			"items":       map[string]any{"type": "string"},
		},
	})
}

func (t *GitCommit) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Message string   `json:"message"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}

	if params.Message == "" {
		return "", fmt.Errorf("commit message is required")
	}

	// Stage files
	addArgs := []string{"add"}
	if len(params.Files) > 0 {
		addArgs = append(addArgs, params.Files...)
	} else {
		addArgs = append(addArgs, "-A")
	}
	addCmd := exec.CommandContext(ctx, "git", addArgs...)
	addCmd.Dir = t.projectRoot
	if out, err := addCmd.CombinedOutput(); err != nil {
		return string(out), fmt.Errorf("git add failed: %w", err)
	}

	// Commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", params.Message)
	commitCmd.Dir = t.projectRoot
	out, err := commitCmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git commit failed: %w", err)
	}
	return string(out), nil
}

// ========== git_status ==========

type GitStatus struct {
	projectRoot string
}

func (t *GitStatus) Name() string        { return "git_status" }
func (t *GitStatus) Description() string { return "查看 Git 仓库状态" }
func (t *GitStatus) InputSchema() json.RawMessage {
	return MakeSchema(map[string]any{})
}

func (t *GitStatus) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--short")
	cmd.Dir = t.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git status failed: %w", err)
	}
	if len(output) == 0 {
		return "working tree clean", nil
	}
	return string(output), nil
}
