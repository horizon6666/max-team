package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/anthropics/anthropic-sdk-go"
)

type SearchCode struct {
	projectRoot string
}

func (t *SearchCode) Name() string        { return "search_code" }
func (t *SearchCode) Description() string { return "在代码库中搜索指定模式的文本" }
func (t *SearchCode) InputSchema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "搜索模式（grep 正则表达式）",
			},
			"file_pattern": map[string]any{
				"type":        "string",
				"description": "文件名过滤（如 *.go），可选",
			},
		},
	}
}

func (t *SearchCode) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Pattern     string `json:"pattern"`
		FilePattern string `json:"file_pattern"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}

	if params.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	args := []string{"-rn", "--color=never"}
	if params.FilePattern != "" {
		args = append(args, "--include="+params.FilePattern)
	}
	args = append(args, params.Pattern, ".")

	cmd := exec.CommandContext(ctx, "grep", args...)
	cmd.Dir = t.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "no matches found", nil
		}
		return string(output), fmt.Errorf("grep failed: %w", err)
	}
	return string(output), nil
}
