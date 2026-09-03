package ai

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/turanmahmudov/masume/internal/agent"
)

// The connection between the tool catalogue and a provider: the schemas sent to a model,
// and the call that runs one tool.

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

// CallToolDefinition runs the tool with this name and returns its result as JSON text for
// the model. An unknown name returns an error message and does not panic, because the model
// sent the name and needs the answer.
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

// writeToolOutput returns the result of a call as text for the model. An HTML character is
// not escaped and stays as the statement wrote it.
func writeToolOutput(answered any) string {
	written := &bytes.Buffer{}
	encoder := json.NewEncoder(written)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(answered); err != nil {
		return `{"error":"the answer of this tool cannot be written as JSON"}`
	}
	return string(bytes.TrimRight(written.Bytes(), "\n"))
}
