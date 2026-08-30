package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordedRequest is one request a provider stand-in was sent.
type recordedRequest struct {
	path    string
	headers http.Header
	body    map[string]any
}

// serveCannedStreams answers each request with the next canned stream, and writes down what it
// was sent.
func serveCannedStreams(t *testing.T, streams ...string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	sent := &[]recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, asked *http.Request) {
			raw, _ := io.ReadAll(asked.Body)
			body := map[string]any{}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("the request is not JSON: %v", err)
			}
			*sent = append(*sent, recordedRequest{
				path: asked.URL.Path, headers: asked.Header.Clone(), body: body,
			})

			at := len(*sent) - 1
			if at >= len(streams) {
				at = len(streams) - 1
			}
			writer.Header().Set("content-type", "text/event-stream")
			_, _ = writer.Write([]byte(streams[at]))
		}))
	t.Cleanup(server.Close)
	return server, sent
}

// buildEvent writes one event of a stream.
func buildEvent(name string, data string) string {
	return "event: " + name + "\ndata: " + data + "\n\n"
}

// The canned answers of Anthropic: one that asks for a call, and one that answers.
var anthropicCallsATool = buildEvent("message_start",
	`{"type":"message_start","message":{"usage":{"input_tokens":100,"output_tokens":1,`+
		`"cache_read_input_tokens":0,"cache_creation_input_tokens":40}}}`) +
	buildEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
	buildEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,`+
			`"delta":{"type":"text_delta","text":"Let me look."}}`) +
	buildEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
	buildEvent("content_block_start",
		`{"type":"content_block_start","index":1,"content_block":`+
			`{"type":"tool_use","id":"toolu_1","name":"list_tables"}}`) +
	buildEvent("content_block_delta",
		`{"type":"content_block_delta","index":1,`+
			`"delta":{"type":"input_json_delta","partial_json":"{\"limit\":5}"}}`) +
	buildEvent("content_block_stop", `{"type":"content_block_stop","index":1}`) +
	buildEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},`+
			`"usage":{"output_tokens":20}}`) +
	buildEvent("message_stop", `{"type":"message_stop"}`)

var anthropicAnswers = buildEvent("message_start",
	`{"type":"message_start","message":{"usage":{"input_tokens":30,"output_tokens":1,`+
		`"cache_read_input_tokens":140,"cache_creation_input_tokens":0}}}`) +
	buildEvent("content_block_start",
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
	buildEvent("content_block_delta",
		`{"type":"content_block_delta","index":0,`+
			`"delta":{"type":"text_delta","text":"Two tables."}}`) +
	buildEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
	buildEvent("message_delta",
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},`+
			`"usage":{"output_tokens":55}}`) +
	buildEvent("message_stop", `{"type":"message_stop"}`)

// The canned answers of OpenAI, in the shapes of its own protocol.
var openaiCallsATool = buildEvent("response.created",
	`{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`) +
	buildEvent("response.content_part.added",
		`{"type":"response.content_part.added","item_id":"msg_1",`+
			`"part":{"type":"output_text","text":""}}`) +
	buildEvent("response.output_text.delta",
		`{"type":"response.output_text.delta","item_id":"msg_1","delta":"Let me look."}`) +
	buildEvent("response.output_item.added",
		`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1",`+
			`"call_id":"call_1","name":"list_tables","arguments":""}}`) +
	buildEvent("response.function_call_arguments.delta",
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1",`+
			`"delta":"{\"limit\":5}"}`) +
	buildEvent("response.output_item.done",
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1",`+
			`"call_id":"call_1","name":"list_tables","arguments":"{\"limit\":5}"}}`) +
	buildEvent("response.completed",
		`{"type":"response.completed","response":{"status":"completed","usage":`+
			`{"input_tokens":100,"input_tokens_details":{"cached_tokens":0},`+
			`"output_tokens":20}}}`)

var openaiAnswers = buildEvent("response.created",
	`{"type":"response.created","response":{"id":"resp_2","status":"in_progress"}}`) +
	buildEvent("response.content_part.added",
		`{"type":"response.content_part.added","item_id":"msg_2",`+
			`"part":{"type":"output_text","text":""}}`) +
	buildEvent("response.output_text.delta",
		`{"type":"response.output_text.delta","item_id":"msg_2","delta":"Two tables."}`) +
	buildEvent("response.completed",
		`{"type":"response.completed","response":{"status":"completed","usage":`+
			`{"input_tokens":170,"input_tokens_details":{"cached_tokens":128},`+
			`"output_tokens":55}}}`)

// buildTestRequest answers one question with one tool, as a chat of a connection sends it.
func buildTestRequest() Request {
	return Request{
		System:   "You are a SQL assistant.",
		Messages: []Message{{Role: RoleUser, Text: "how many orders are unpaid"}},
		Tools: []ToolSchema{{
			Name: "list_tables", Description: "List the tables.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		}},
	}
}

// collectRun answers what a whole run reported, with the calls it made.
func collectRun(t *testing.T, model Model, request Request) (RunResult, []string, string) {
	t.Helper()
	text := strings.Builder{}
	steps := []string{}
	result, err := RunChat(context.Background(), model, request, RunHooks{
		StartTextBlock: func() {
			if text.Len() > 0 {
				text.WriteString("\n\n")
			}
		},
		AppendText:     func(delta string) { text.WriteString(delta) },
		StartToolStep:  func(label string) { steps = append(steps, label) },
		FinishToolStep: func() {},
		CallTool: func(_ context.Context, name string, input map[string]any) string {
			return `{"called":"` + name + `","limit":` +
				writeToolOutput(input["limit"]) + `}`
		},
		LogEvent: func(string) {},
	})
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	return result, steps, text.String()
}

func TestAnthropicAsksAndRunsWhatItIsAskedFor(t *testing.T) {
	server, sent := serveCannedStreams(t, anthropicCallsATool, anthropicAnswers)
	model := openAnthropicModel("claude-opus-5", "probe-key", server.URL+"/v1")

	result, steps, written := collectRun(t, model, buildTestRequest())
	if written != "Let me look.\n\nTwo tables." {
		t.Errorf("the reply reads %q", written)
	}
	if len(steps) != 1 || steps[0] != "listing the tables" {
		t.Errorf("the steps are %v", steps)
	}
	if result.FinishReason != FinishStop {
		t.Errorf("the run stopped for %q", result.FinishReason)
	}
	// The counts of both requests are added, and the input of this protocol leaves out what
	// it read from its cache.
	wanted := Usage{InputTokens: 140 + 170, OutputTokens: 75, CachedInputTokens: 140}
	if result.Usage != wanted {
		t.Errorf("the run spent %+v, wanted %+v", result.Usage, wanted)
	}

	if len(*sent) != 2 {
		t.Fatalf("the model was asked %d times, wanted twice", len(*sent))
	}
	first := (*sent)[0]
	if first.path != "/v1/messages" {
		t.Errorf("the request went to %s", first.path)
	}
	if first.headers.Get("x-api-key") != "probe-key" ||
		first.headers.Get("anthropic-version") != anthropicVersion ||
		first.headers.Get("anthropic-beta") != strictSchemaBeta {
		t.Errorf("the headers read %v", first.headers)
	}
	if first.body["max_tokens"] != float64(largeOutputTokens) {
		t.Errorf("the request allows %v tokens", first.body["max_tokens"])
	}
	if choice, _ := first.body["tool_choice"].(map[string]any); choice["type"] != "auto" {
		t.Errorf("the request names the tool choice %v", first.body["tool_choice"])
	}
	// The system prompt is one block of text, and the last block of the last message is
	// marked, so everything before it is read back from the cache.
	system, _ := first.body["system"].([]any)
	if len(system) != 1 {
		t.Fatalf("the system prompt is %v", first.body["system"])
	}
	if held, _ := system[0].(map[string]any); held["cache_control"] != nil {
		t.Errorf("the system prompt carries a mark of its own")
	}
	if mark := findAnthropicCacheMark(t, first.body); mark == nil {
		t.Error("the last block of the request carries no mark")
	}

	// The second request carries the call and its answer, and only its last block is marked.
	second := (*sent)[1]
	messages, _ := second.body["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("the second request holds %d messages, wanted three", len(messages))
	}
	assistant, _ := messages[1].(map[string]any)
	blocks, _ := assistant["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("the assistant turn holds %v", assistant["content"])
	}
	call, _ := blocks[1].(map[string]any)
	if call["type"] != "tool_use" || call["id"] != "toolu_1" || call["name"] != "list_tables" {
		t.Errorf("the call reads %v", call)
	}
	answer, _ := messages[2].(map[string]any)
	if answer["role"] != RoleUser {
		t.Errorf("the answer of the call is a %v turn", answer["role"])
	}
	answered, _ := answer["content"].([]any)
	held, _ := answered[0].(map[string]any)
	if held["type"] != "tool_result" || held["tool_use_id"] != "toolu_1" {
		t.Errorf("the answer reads %v", held)
	}
	if first, _ := messages[0].(map[string]any); findMark(first) != nil {
		t.Error("a message of an earlier step keeps its mark")
	}
}

// findAnthropicCacheMark answers the mark on the last block of the last message.
func findAnthropicCacheMark(t *testing.T, body map[string]any) any {
	t.Helper()
	messages, _ := body["messages"].([]any)
	if len(messages) == 0 {
		return nil
	}
	last, _ := messages[len(messages)-1].(map[string]any)
	return findMark(last)
}

// findMark answers the mark on the last block of one message.
func findMark(message map[string]any) any {
	blocks, _ := message["content"].([]any)
	if len(blocks) == 0 {
		return nil
	}
	last, _ := blocks[len(blocks)-1].(map[string]any)
	return last["cache_control"]
}

func TestOpenaiAsksAndRunsWhatItIsAskedFor(t *testing.T) {
	server, sent := serveCannedStreams(t, openaiCallsATool, openaiAnswers)
	model := openOpenaiModel("gpt-5", "probe-key", server.URL+"/v1", "masume/shop")

	result, steps, written := collectRun(t, model, buildTestRequest())
	if written != "Let me look.\n\nTwo tables." {
		t.Errorf("the reply reads %q", written)
	}
	if len(steps) != 1 || steps[0] != "listing the tables" {
		t.Errorf("the steps are %v", steps)
	}
	if result.FinishReason != FinishStop {
		t.Errorf("the run stopped for %q", result.FinishReason)
	}
	wanted := Usage{InputTokens: 270, OutputTokens: 75, CachedInputTokens: 128}
	if result.Usage != wanted {
		t.Errorf("the run spent %+v, wanted %+v", result.Usage, wanted)
	}

	if len(*sent) != 2 {
		t.Fatalf("the model was asked %d times, wanted twice", len(*sent))
	}
	first := (*sent)[0]
	if first.path != "/v1/responses" {
		t.Errorf("the request went to %s", first.path)
	}
	if first.headers.Get("authorization") != "Bearer probe-key" {
		t.Errorf("the headers read %v", first.headers)
	}
	if first.body["prompt_cache_key"] != "masume/shop" {
		t.Errorf("the cache key reads %v", first.body["prompt_cache_key"])
	}
	if first.body["tool_choice"] != "auto" {
		t.Errorf("the request names the tool choice %v", first.body["tool_choice"])
	}
	// The turn that opens the request carries its text plainly.
	input, _ := first.body["input"].([]any)
	opening, _ := input[0].(map[string]any)
	if opening["role"] != "developer" || opening["content"] != "You are a SQL assistant." {
		t.Errorf("the opening turn reads %v", opening)
	}
	tools, _ := first.body["tools"].([]any)
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "list_tables" {
		t.Errorf("the tool reads %v", tool)
	}

	// The second request carries what the model wrote, then the call and its answer as items
	// of their own.
	second, _ := (*sent)[1].body["input"].([]any)
	if len(second) != 5 {
		t.Fatalf("the second request holds %d items, wanted five", len(second))
	}
	wrote, _ := second[2].(map[string]any)
	parts, _ := wrote["content"].([]any)
	part, _ := parts[0].(map[string]any)
	if wrote["role"] != RoleAssistant || part["type"] != openaiOutputText {
		t.Errorf("what the model wrote reads %v", wrote)
	}
	call, _ := second[3].(map[string]any)
	if call["type"] != "function_call" || call["call_id"] != "call_1" ||
		call["arguments"] != `{"limit":5}` {
		t.Errorf("the call reads %v", call)
	}
	answered, _ := second[4].(map[string]any)
	if answered["type"] != "function_call_output" || answered["call_id"] != "call_1" {
		t.Errorf("the answer of the call reads %v", answered)
	}
}

func TestARefusedRequestReadsAsTheProviderWroteIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(
				`{"type":"error","error":{"type":"invalid_request_error",` +
					`"message":"max_tokens: 1 is too small"}}`))
		}))
	defer server.Close()

	model := openAnthropicModel("claude-opus-5", "probe-key", server.URL+"/v1")
	_, err := model.Stream(context.Background(), buildTestRequest(), func(Event) {})
	if err == nil || err.Error() != "max_tokens: 1 is too small" {
		t.Errorf("the failure reads %v", err)
	}
}

func TestResolveVersionedBaseURL(t *testing.T) {
	cases := []struct{ given, wanted string }{
		{"http://127.0.0.1:8765", "http://127.0.0.1:8765/v1"},
		{"http://127.0.0.1:8765/", "http://127.0.0.1:8765/v1"},
		{"http://127.0.0.1:8765/v1", "http://127.0.0.1:8765/v1"},
		{"http://127.0.0.1:8765/V1/", "http://127.0.0.1:8765/V1"},
		{"https://proxy/anthropic", "https://proxy/anthropic/v1"},
	}
	for _, held := range cases {
		if answered := ResolveVersionedBaseURL(held.given); answered != held.wanted {
			t.Errorf("%q became %q, wanted %q", held.given, answered, held.wanted)
		}
	}
}

func TestResolveModelCapabilities(t *testing.T) {
	cases := []struct {
		model  string
		limit  int
		strict bool
	}{
		{"claude-opus-5", largeOutputTokens, true},
		{"claude-sonnet-5", largeOutputTokens, true},
		{"claude-haiku-4-5-20251001", mediumOutputTokens, true},
		{"claude-opus-4-1", smallerOutputTokens, true},
		{"claude-sonnet-4-0", mediumOutputTokens, false},
		{"claude-opus-4-0", smallerOutputTokens, false},
		{"claude-3-haiku-20240307", leastOutputTokens, false},
		{"claude-3-5-sonnet", leastOutputTokens, false},
		{"anthropic.claude-v2:1", leastOutputTokens, false},
		{"claude-instant-1", leastOutputTokens, false},
		{"claude-something-new", largeOutputTokens, true},
		{"a-model-of-a-proxy", leastOutputTokens, false},
	}
	for _, held := range cases {
		answered := resolveModelCapabilities(held.model)
		if answered.outputLimit != held.limit ||
			answered.readsSchemasStrictly != held.strict {
			t.Errorf("%s answers at most %d tokens and reads strictly %v, wanted %d and %v",
				held.model, answered.outputLimit, answered.readsSchemasStrictly,
				held.limit, held.strict)
		}
	}
}

func TestFindEmptyReplyProblem(t *testing.T) {
	if problem := FindEmptyReplyProblem(12, FinishStop); problem != "" {
		t.Errorf("a reply that arrived was reported: %s", problem)
	}
	if problem := FindEmptyReplyProblem(0, FinishStop); problem != "" {
		t.Errorf("a model that stopped with nothing to say was reported: %s", problem)
	}
	if problem := FindEmptyReplyProblem(0, FinishToolCalls); !strings.Contains(
		problem, "ran out of turns after 25 tool calls") {
		t.Errorf("the problem reads %q", problem)
	}
	if problem := FindEmptyReplyProblem(0, FinishContentFilter); problem !=
		"the model answered nothing (content-filter)" {
		t.Errorf("the problem reads %q", problem)
	}
}

func TestARunThatWasStoppedWritesNothingMore(t *testing.T) {
	server, sent := serveCannedStreams(t, anthropicCallsATool, anthropicAnswers)
	model := openAnthropicModel("claude-opus-5", "probe-key", server.URL+"/v1")

	// The run is stopped as soon as the first call is asked for.
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	written := strings.Builder{}
	result, err := RunChat(ctx, model, buildTestRequest(), RunHooks{
		StartTextBlock: func() {},
		AppendText:     func(delta string) { written.WriteString(delta) },
		StartToolStep:  func(string) { stop() },
		FinishToolStep: func() {},
		CallTool:       func(context.Context, string, map[string]any) string { return "{}" },
		LogEvent:       func(string) {},
	})
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if written.String() != "Let me look." {
		t.Errorf("the reply reads %q, wanted only what arrived before the stop",
			written.String())
	}
	if len(*sent) != 1 {
		t.Errorf("a stopped run asked the model %d times", len(*sent))
	}
	// The tokens of the request that ran are still counted, because they were spent.
	if result.Usage.InputTokens == 0 {
		t.Error("a stopped run counted no tokens")
	}
}

// The chat proposes a statement the connected server takes. A server that has no SQL was
// told to write SQL, and the reply was useless on it.
func TestBuildChatSystemPromptWritesTheLanguageOfTheServer(t *testing.T) {
	built := BuildChatSystemPrompt(ChatPromptSource{
		DialectName: "mongodb", DefaultSchema: "shop",
		Language: StatementLanguage{
			Name: "MongoDB shell calls", FenceTag: "js",
			Example: "One statement is one call of the shell.",
		},
	})
	for _, wanted := range []string{
		"write correct MongoDB shell calls",
		"opened with ```js",
		"One statement is one call of the shell.",
	} {
		if !strings.Contains(built, wanted) {
			t.Errorf("the prompt does not say %q:\n%s", wanted, built)
		}
	}
	if strings.Contains(built, "```sql") {
		t.Errorf("the prompt still asks for a SQL block:\n%s", built)
	}
}

// A server that names no language of its own is a SQL server, which is what every engine
// but two is.
func TestBuildChatSystemPromptFallsBackToSql(t *testing.T) {
	built := BuildChatSystemPrompt(ChatPromptSource{
		DialectName: "postgres", DefaultSchema: "public",
	})
	if !strings.Contains(built, "write correct SQL") || !strings.Contains(built, "```sql") {
		t.Errorf("the prompt does not ask for SQL:\n%s", built)
	}
}

// The reply of a model arrives as a stream that is read as it comes, so the client carries no
// deadline of its own. A whole-request timeout cuts a long reply in the middle.
func TestTheStreamingClientCarriesNoDeadline(t *testing.T) {
	if streamingClient.Timeout != 0 {
		t.Errorf("the client cuts a request after %s", streamingClient.Timeout)
	}
}

// A data line of a stream loses the one space that follows the colon, and keeps every space
// after it, because that is what the event stream says.
func TestReadServerEventsKeepsTheDataAfterTheFirstSpace(t *testing.T) {
	stream := "event: delta\ndata:  held\ndata: more\n\ndata:{\"a\":1}\n\n"
	read := []string{}
	if err := readServerEvents(strings.NewReader(stream), func(event serverEvent) error {
		read = append(read, event.data)
		return nil
	}); err != nil {
		t.Fatalf("the stream failed: %v", err)
	}
	if len(read) != 2 || read[0] != " held\nmore" || read[1] != `{"a":1}` {
		t.Errorf("the events read as %q", read)
	}
}
