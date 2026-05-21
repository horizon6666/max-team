package tool

import (
	"context"
	"encoding/json"
)

type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

func MakeSchema(properties map[string]any) json.RawMessage {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	b, _ := json.Marshal(schema)
	return b
}
