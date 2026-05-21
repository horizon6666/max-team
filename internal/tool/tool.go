package tool

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

type Tool interface {
	Name() string
	Description() string
	InputSchema() anthropic.ToolInputSchemaParam
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

func ToAnthropicTools(tools []Tool) []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, len(tools))
	for i, t := range tools {
		tp := anthropic.ToolParam{
			Name:        t.Name(),
			Description: anthropic.String(t.Description()),
			InputSchema: t.InputSchema(),
		}
		params[i] = anthropic.ToolUnionParam{OfTool: &tp}
	}
	return params
}
