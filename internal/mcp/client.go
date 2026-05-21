package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/horizon6666/max-team/internal/config"
	"github.com/horizon6666/max-team/internal/tool"
)

type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID int
	tools  []MCPToolDef
}

func NewClient(cfg config.MCPServerConfig) (*Client, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mcp server: %w", err)
	}

	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		nextID: 1,
	}

	if err := c.initialize(); err != nil {
		c.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	if err := c.discoverTools(); err != nil {
		c.Close()
		return nil, fmt.Errorf("discover tools: %w", err)
	}

	return c, nil
}

func (c *Client) initialize() error {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo:      ClientInfo{Name: "max-team", Version: "0.1.0"},
	}
	paramsJSON, _ := json.Marshal(params)

	var result InitializeResult
	if err := c.call("initialize", paramsJSON, &result); err != nil {
		return err
	}

	log.Printf("[mcp] connected to %s %s", result.ServerInfo.Name, result.ServerInfo.Version)
	return nil
}

func (c *Client) discoverTools() error {
	var result ToolsListResult
	if err := c.call("tools/list", nil, &result); err != nil {
		return err
	}
	c.tools = result.Tools
	log.Printf("[mcp] discovered %d tools", len(c.tools))
	return nil
}

func (c *Client) Tools() []tool.Tool {
	result := make([]tool.Tool, len(c.tools))
	for i, def := range c.tools {
		result[i] = &MCPTool{client: c, def: def}
	}
	return result
}

func (c *Client) CallTool(name string, args json.RawMessage) (string, error) {
	params := CallToolParams{Name: name, Arguments: args}
	paramsJSON, _ := json.Marshal(params)

	var result CallToolResult
	if err := c.call("tools/call", paramsJSON, &result); err != nil {
		return "", err
	}

	var output string
	for _, content := range result.Content {
		if content.Type == "text" {
			output += content.Text
		}
	}

	if result.IsError {
		return output, fmt.Errorf("tool error: %s", output)
	}
	return output, nil
}

func (c *Client) Close() {
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
}

func (c *Client) call(method string, params json.RawMessage, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID,
		Method:  method,
		Params:  params,
	}
	c.nextID++

	data, _ := json.Marshal(req)
	data = append(data, '\n')

	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	if result != nil && resp.Result != nil {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

// MCPTool wraps an MCP tool definition as a tool.Tool
type MCPTool struct {
	client *Client
	def    MCPToolDef
}

func (t *MCPTool) Name() string        { return t.def.Name }
func (t *MCPTool) Description() string { return t.def.Description }
func (t *MCPTool) InputSchema() anthropic.ToolInputSchemaParam {
	var schema anthropic.ToolInputSchemaParam
	json.Unmarshal(t.def.InputSchema, &schema)
	return schema
}

func (t *MCPTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	return t.client.CallTool(t.def.Name, input)
}
