package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// ========== read_file ==========

type ReadFile struct {
	projectRoot string
	sandbox     bool
}

func (t *ReadFile) Name() string        { return "read_file" }
func (t *ReadFile) Description() string { return "读取指定路径的文件内容" }
func (t *ReadFile) InputSchema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "文件路径（相对于项目根目录）",
			},
		},
	}
}

func (t *ReadFile) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}

	absPath, err := t.resolve(params.Path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (t *ReadFile) resolve(path string) (string, error) {
	absRoot, _ := filepath.Abs(t.projectRoot)
	absPath, _ := filepath.Abs(filepath.Join(t.projectRoot, path))
	if t.sandbox && !strings.HasPrefix(absPath, absRoot) {
		return "", fmt.Errorf("access denied: path outside project root")
	}
	return absPath, nil
}

// ========== write_file ==========

type WriteFile struct {
	projectRoot     string
	sandbox         bool
	blockedPatterns []string
	writePatterns   []string
}

func (t *WriteFile) Name() string        { return "write_file" }
func (t *WriteFile) Description() string { return "写入内容到指定路径的文件" }
func (t *WriteFile) InputSchema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "文件路径（相对于项目根目录）",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "要写入的文件内容",
			},
		},
	}
}

func (t *WriteFile) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}

	absRoot, _ := filepath.Abs(t.projectRoot)
	absPath, _ := filepath.Abs(filepath.Join(t.projectRoot, params.Path))

	if t.sandbox && !strings.HasPrefix(absPath, absRoot) {
		return "", fmt.Errorf("access denied: path outside project root")
	}

	base := filepath.Base(params.Path)
	for _, pattern := range t.blockedPatterns {
		if matched, _ := filepath.Match(pattern, base); matched {
			return "", fmt.Errorf("access denied: %s matches blocked pattern %s", params.Path, pattern)
		}
	}

	if len(t.writePatterns) > 0 {
		allowed := false
		for _, pattern := range t.writePatterns {
			if matched, _ := filepath.Match(pattern, base); matched {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("access denied: %s not in write whitelist", params.Path)
		}
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(absPath, []byte(params.Content), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("written %d bytes to %s", len(params.Content), params.Path), nil
}

// ========== list_dir ==========

type ListDir struct {
	projectRoot string
	sandbox     bool
}

func (t *ListDir) Name() string        { return "list_dir" }
func (t *ListDir) Description() string { return "列出指定目录的内容" }
func (t *ListDir) InputSchema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "目录路径（相对于项目根目录）",
			},
		},
	}
}

func (t *ListDir) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}

	absRoot, _ := filepath.Abs(t.projectRoot)
	absPath, _ := filepath.Abs(filepath.Join(t.projectRoot, params.Path))

	if t.sandbox && !strings.HasPrefix(absPath, absRoot) {
		return "", fmt.Errorf("access denied: path outside project root")
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, e := range entries {
		info, _ := e.Info()
		if e.IsDir() {
			fmt.Fprintf(&sb, "d %s/\n", e.Name())
		} else if info != nil {
			fmt.Fprintf(&sb, "f %s (%d bytes)\n", e.Name(), info.Size())
		} else {
			fmt.Fprintf(&sb, "f %s\n", e.Name())
		}
	}
	return sb.String(), nil
}
