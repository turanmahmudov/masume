package ai

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/turanmahmudov/masume/internal/agent"
)

// The one place the catalogue of the tools meets a provider: the schemas a model is told
// about, and the call that runs one of them.

// BuildToolSchemas returns the catalogue as a provider is told about it.
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

// CallToolDefinition runs the call of this name and returns what it said, as the JSON text a
// model reads. A name the catalogue does not hold is answered, not thrown, because the model
// asked for it and must be told.
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

// writeToolOutput writes what a call answered as the text the model reads. A character a
// browser cares about is left as the statement wrote it.
func writeToolOutput(answered any) string {
	written := &bytes.Buffer{}
	encoder := json.NewEncoder(written)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(answered); err != nil {
		return `{"error":"the answer of this tool cannot be written as JSON"}`
	}
	return string(bytes.TrimRight(written.Bytes(), "\n"))
}
