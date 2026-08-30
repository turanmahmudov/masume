package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// The protocol itself: JSON-RPC 2.0, one message per line, over standard input and standard
// output. Nothing else may write to standard output while this runs.

const jsonRPCVersion = "2.0"

// The codes JSON-RPC gives a failure.
const (
	parseError     = -32700
	invalidRequest = -32600
	methodNotFound = -32601
	invalidParams  = -32602
	internalError  = -32603
)

// knownProtocolVersions holds the versions of the protocol this server speaks, newest first.
var knownProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// ServerInfo is the name and version the server reports to a client.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// resultMessage is the answer to a call that worked.
type resultMessage struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result"`
}

// errorMessage is the answer to a call that did not work.
type errorMessage struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      any          `json:"id"`
	Error   errorContent `json:"error"`
}

type errorContent struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// requestMessage is a request the server sends the other way, which is only the question it
// asks its client.
type requestMessage struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// sessionStart is what the server returns at initialize.
type sessionStart struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	Tools toolCapabilities `json:"tools"`
}

type toolCapabilities struct {
	ListChanged bool `json:"listChanged"`
}

// toolListing is every tool, as tools/list describes them.
type toolListing struct {
	Tools []describedTool `json:"tools"`
}

type describedTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// toolResult is what a tool answered: text, and whether the call failed.
type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ResponderDeps is everything the protocol needs to answer.
type ResponderDeps struct {
	Tools []Tool
	Info  ServerInfo
	// Asker reads the capabilities at initialize, and asks the user of the client.
	Asker    *Asker
	LogEvent func(message string)
}

// Responder returns one message at a time, and knows nothing of the transport.
type Responder struct {
	deps ResponderDeps
}

// CreateResponder builds the answering side of the protocol.
func CreateResponder(deps ResponderDeps) *Responder {
	return &Responder{deps: deps}
}

func buildResult(id any, result any) any {
	return resultMessage{JSONRPC: jsonRPCVersion, ID: id, Result: result}
}

func buildError(id any, code int, message string) any {
	return errorMessage{
		JSONRPC: jsonRPCVersion, ID: id,
		Error: errorContent{Code: code, Message: message},
	}
}

// buildToolResult writes what a tool answered, and reports whether the call failed.
func buildToolResult(text string, failed bool) toolResult {
	return toolResult{
		Content: []toolContent{{Type: "text", Text: text}}, IsError: failed,
	}
}

// castToObject reads a value as the named fields of an object, and returns no fields for
// anything else.
func castToObject(value any) map[string]any {
	named, is := value.(map[string]any)
	if !is {
		return map[string]any{}
	}
	return named
}

// encodeJSON writes a value as JSON, with an indent where the reader is a person. A character
// a browser cares about is left as it was written, because a statement is read by a model.
func encodeJSON(value any, indent string) (string, error) {
	written := &bytes.Buffer{}
	encoder := json.NewEncoder(written)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimRight(written.String(), "\n"), nil
}

// buildJSONLine writes one message as a line. Nothing this server sends can fail to be
// written, so a failure returns with the protocol error rather than with nothing.
func buildJSONLine(message any) string {
	line, err := encodeJSON(message, "")
	if err == nil {
		return line
	}
	return `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,` +
		`"message":"the answer cannot be written as JSON"}}`
}

// resolveProtocolVersion returns the version the client asked for, where this server knows it.
func resolveProtocolVersion(asked any) string {
	written, is := asked.(string)
	if is {
		if slices.Contains(knownProtocolVersions, written) {
			return written
		}
	}
	return knownProtocolVersions[0]
}

// The kinds one message can be, once its id and its method are read.
const (
	// messageMalformed is anything that is not a JSON object.
	messageMalformed = iota
	// messageMethodless names no method, which the client sends to answer the server.
	messageMethodless
	// messageCall names a method to answer.
	messageCall
)

// incoming is one message, read far enough to know what to do with it.
type incoming struct {
	kind int
	id   any
	// wantsAnswer is false for a notification, which is handled and not answered.
	wantsAnswer bool
	method      string
	params      map[string]any
	named       map[string]any
}

func readIncomingMessage(message any) incoming {
	named, is := message.(map[string]any)
	if !is {
		return incoming{kind: messageMalformed}
	}

	held := named["id"]
	var id any
	switch held.(type) {
	case string, float64:
		id = held
	}
	wantsAnswer := held != nil

	method, is := named["method"].(string)
	if !is {
		return incoming{
			kind: messageMethodless, id: id, wantsAnswer: wantsAnswer, named: named,
		}
	}
	return incoming{
		kind: messageCall, id: id, wantsAnswer: wantsAnswer, method: method,
		params: castToObject(named["params"]),
	}
}

// AnswerMessage returns one message, and nothing for a notification or an answer, which need
// no reply.
func (responder *Responder) AnswerMessage(ctx context.Context, message any) any {
	held := readIncomingMessage(message)
	switch held.kind {
	case messageMalformed:
		return buildError(nil, invalidRequest, "a message must be a JSON object")
	case messageMethodless:
		return responder.answerMethodless(held)
	}
	return responder.answerSafely(ctx, held)
}

func (responder *Responder) answerMethodless(held incoming) any {
	// The client replies to a question the server asked.
	if responder.deps.Asker.ReceiveAnswer(held.named) {
		return nil
	}
	if !held.wantsAnswer {
		return nil
	}
	return buildError(held.id, invalidRequest, "a message needs a method")
}

func (responder *Responder) answerSafely(ctx context.Context, held incoming) any {
	answer, err := responder.answerCall(ctx, held.id, held.method, held.params)
	if err == nil {
		if !held.wantsAnswer {
			return nil
		}
		return answer
	}

	message := db.DescribeError(err)
	responder.deps.LogEvent("! " + held.method + " " + message)
	if !held.wantsAnswer {
		return nil
	}
	code := internalError
	refusal := &Refusal{}
	if errors.As(err, &refusal) {
		code = invalidParams
	}
	return buildError(held.id, code, message)
}

func (responder *Responder) answerCall(
	ctx context.Context, id any, method string, params map[string]any,
) (any, error) {
	switch method {
	case "initialize":
		return buildResult(id, responder.startSession(params)), nil
	case "ping":
		return buildResult(id, map[string]any{}), nil
	case "tools/list":
		return buildResult(id, responder.describeTools()), nil
	case "tools/call":
		answered, err := responder.callTool(ctx, params)
		if err != nil {
			return nil, err
		}
		return buildResult(id, answered), nil
	}
	return buildError(id, methodNotFound, fmt.Sprintf("no method named %q", method)), nil
}

func (responder *Responder) startSession(params map[string]any) sessionStart {
	responder.deps.Asker.RememberClient(params["capabilities"])
	client, named := castToObject(params["clientInfo"])["name"].(string)
	if !named {
		client = "a client"
	}
	responder.deps.LogEvent("> initialize " + client)
	return sessionStart{
		ProtocolVersion: resolveProtocolVersion(params["protocolVersion"]),
		Capabilities:    serverCapabilities{Tools: toolCapabilities{ListChanged: false}},
		ServerInfo:      responder.deps.Info,
	}
}

func (responder *Responder) describeTools() toolListing {
	described := make([]describedTool, 0, len(responder.deps.Tools))
	for _, tool := range responder.deps.Tools {
		described = append(described, describedTool{
			Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema,
		})
	}
	return toolListing{Tools: described}
}

func (responder *Responder) callTool(
	ctx context.Context, params map[string]any,
) (toolResult, error) {
	name, _ := params["name"].(string)
	tool := responder.findTool(name)
	if tool == nil {
		return toolResult{}, refuse("no tool named %q", name)
	}

	given := castToObject(params["arguments"])
	asked, _ := encodeJSON(given, "")
	responder.deps.LogEvent("> tool " + tool.Name + " " + core.CutForLog(asked))

	answered, err := tool.Call(ctx, given)
	if err == nil {
		var written string
		written, err = encodeJSON(answered, "  ")
		if err == nil {
			responder.deps.LogEvent("< tool " + tool.Name + " " + core.CutForLog(written))
			return buildToolResult(written, false), nil
		}
	}
	// A refusal and a failure go into the answer, not into a protocol error, so the agent can
	// read the reason.
	message := db.DescribeError(err)
	responder.deps.LogEvent("! tool " + tool.Name + " " + message)
	return buildToolResult(message, true), nil
}

func (responder *Responder) findTool(name string) *Tool {
	for at := range responder.deps.Tools {
		if responder.deps.Tools[at].Name == name {
			return &responder.deps.Tools[at]
		}
	}
	return nil
}
