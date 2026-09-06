package ai

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/turanmahmudov/masume/internal/agent"
)

// Provider tool schemas and execution.

// BuildToolSchemas returns the catalogue in the form sent to a provider.
func BuildToolSchemas(definitions []agent.ToolDefinition) []ToolSchema {
	schemas := make([]ToolSchema, 0, len(definitions))
	for _, definition := range definitions {
		schemas = append(schemas, ToolSchema{
			Name: definition.Name, Description: definition.Description,
			InputSchema: definition.InputSchema,
		})
	}
	return schemas
}

// CallToolDefinition runs a named tool and returns JSON, including an error for unknown tools.
func CallToolDefinition(
	ctx context.Context, definitions []agent.ToolDefinition, deps agent.ToolDeps,
	name string, input map[string]any,
) string {
	for _, definition := range definitions {
		if definition.Name != name {
			continue
		}
		return writeToolOutput(definition.Call(ctx, deps, input))
	}
	return writeToolOutput(map[string]any{"error": "no tool named " + name})
}

// writeToolOutput encodes a tool response as JSON without HTML escaping.
func writeToolOutput(answered any) string {
	written := &bytes.Buffer{}
	encoder := json.NewEncoder(written)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(answered); err != nil {
		return `{"error":"cannot encode the tool result as JSON"}`
	}
	return string(bytes.TrimRight(written.Bytes(), "\n"))
}
