package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/agent"
)

// buildTestResponder returns a responder with one tool, and its log lines.
func buildTestResponder(call func(input map[string]any) (any, error)) (*Responder, *[]string) {
	written := &[]string{}
	asker := CreateAsker(func(message string) { *written = append(*written, message) })
	// The transport of the server has a writer before the first message of a client.
	asker.AttachWriter(func(any) {})
	return CreateResponder(ResponderDeps{
		Tools: []Tool{{
			Name: "one_tool", Description: "a tool", InputSchema: agent.BuildEmptySchema(),
			Call: func(_ context.Context, input map[string]any) (any, error) {
				return call(input)
			},
		}},
		Info:     ServerInfo{Name: "masume", Version: "0.1.0"},
		Asker:    asker,
		LogEvent: func(message string) { *written = append(*written, message) },
	}), written
}

// answerOne parses one message as JSON and returns the answer as the line a client reads.
func answerOne(t *testing.T, responder *Responder, written string) string {
	t.Helper()
	var message any
	if err := json.Unmarshal([]byte(written), &message); err != nil {
		t.Fatalf("the message %q is not JSON: %v", written, err)
	}
	answer := responder.AnswerMessage(context.Background(), message)
	if answer == nil {
		return ""
	}
	return buildJSONLine(answer)
}

func TestAnswerEveryMethod(t *testing.T) {
	responder, _ := buildTestResponder(func(map[string]any) (any, error) {
		return map[string]any{"read": "a < b"}, nil
	})

	cases := []struct{ asked, wanted string }{
		{`{"jsonrpc":"2.0","id":1,"method":"ping"}`, `{"jsonrpc":"2.0","id":1,"result":{}}`},
		{`{"jsonrpc":"2.0","id":"a","method":"no/such"}`,
			`{"jsonrpc":"2.0","id":"a","error":{"code":-32601,` +
				`"message":"no method named \"no/such\""}}`},
		{`{"jsonrpc":"2.0","method":"notifications/initialized"}`, ""},
		{`{"jsonrpc":"2.0","id":2}`,
			`{"jsonrpc":"2.0","id":2,"error":{"code":-32600,` +
				`"message":"a message needs a method"}}`},
		{`[1,2]`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,` +
			`"message":"a message must be a JSON object"}}`},
		{`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nope"}}`,
			`{"jsonrpc":"2.0","id":3,"error":{"code":-32602,` +
				`"message":"no tool named \"nope\""}}`},
		// An HTML character stays as the statement wrote it.
		{`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"one_tool"}}`,
			`{"jsonrpc":"2.0","id":4,"result":{"content":[{"type":"text",` +
				`"text":"{\n  \"read\": \"a < b\"\n}"}]}}`},
	}
	for _, held := range cases {
		answered := answerOne(t, responder, held.asked)
		if answered != held.wanted {
			t.Errorf("%s\n  gave  %s\n  wanted %s", held.asked, answered, held.wanted)
		}
	}
}

func TestInitializeReportsTheVersionAsked(t *testing.T) {
	responder, written := buildTestResponder(nil)
	answered := answerOne(t, responder,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":`+
			`{"protocolVersion":"2024-11-05","capabilities":{"elicitation":{}},`+
			`"clientInfo":{"name":"probe"}}}`)
	wanted := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05",` +
		`"capabilities":{"tools":{"listChanged":false}},` +
		`"serverInfo":{"name":"masume","version":"0.1.0"}}}`
	if answered != wanted {
		t.Errorf("initialize answered\n  %s\n  wanted %s", answered, wanted)
	}
	if !responder.deps.Asker.CanAsk() {
		t.Error("a client that says it can ask its user was not remembered")
	}
	if len(*written) != 1 || (*written)[0] != "> initialize probe" {
		t.Errorf("the log holds %v", *written)
	}

	// An unknown version falls back to the newest version this server supports.
	answered = answerOne(t, responder,
		`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"1999"}}`)
	if !strings.Contains(answered, `"protocolVersion":"2025-06-18"`) {
		t.Errorf("an unknown version answered %s", answered)
	}
}

func TestAFailedCallAnswersWithTheReason(t *testing.T) {
	responder, written := buildTestResponder(func(map[string]any) (any, error) {
		return nil, refuse("this connection is read-only")
	})
	answered := answerOne(t, responder,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"one_tool"}}`)
	wanted := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text",` +
		`"text":"this connection is read-only"}],"isError":true}}`
	if answered != wanted {
		t.Errorf("the answer reads\n  %s\n  wanted %s", answered, wanted)
	}
	if len(*written) != 2 || (*written)[1] != "! tool one_tool this connection is read-only" {
		t.Errorf("the log holds %v", *written)
	}
}

func TestToolsListDescribesEveryTool(t *testing.T) {
	responder, _ := buildTestResponder(nil)
	answered := answerOne(t, responder, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if !strings.Contains(answered, `"name":"one_tool"`) ||
		!strings.Contains(answered, `"inputSchema":{"$schema"`) {
		t.Errorf("the listing reads %s", answered)
	}
}

func TestServeOverStdioAnswersEveryLine(t *testing.T) {
	responder, _ := buildTestResponder(func(map[string]any) (any, error) {
		return map[string]any{"ran": true}, nil
	})
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		"",
		"not json",
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"one_tool"}}`,
	}, "\n")

	answers := []string{}
	ServeOverStdio(context.Background(), responder, strings.NewReader(input),
		func(line string) { answers = append(answers, line) })

	if len(answers) != 3 {
		t.Fatalf("the server wrote %v", answers)
	}
	if answers[0] != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Errorf("the first answer reads %s", answers[0])
	}
	if !strings.Contains(answers[1], `"code":-32700`) {
		t.Errorf("a line that is not JSON answered %s", answers[1])
	}
	if !strings.Contains(answers[2], `"ran\": true`) {
		t.Errorf("the call answered %s", answers[2])
	}
}

// A message without a line break would grow the buffer of the reader until the machine has
// no memory left, and the client would control the memory of this process.
func TestServeOverStdioRefusesAMessageAboveTheLimit(t *testing.T) {
	responder, _ := buildTestResponder(func(map[string]any) (any, error) {
		return map[string]any{"ran": true}, nil
	})
	// One valid line, far above the limit of the server.
	long := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` +
		strings.Repeat("x", maxMessageBytes) + `"}}`

	answers := []string{}
	ServeOverStdio(context.Background(), responder, strings.NewReader(long),
		func(line string) { answers = append(answers, line) })

	if len(answers) != 1 {
		t.Fatalf("the server wrote %v", answers)
	}
	if !strings.Contains(answers[0], "at most") {
		t.Errorf("the answer reads %s", answers[0])
	}
}

// A long message below the limit is read complete, over several buffer reads, and the next
// message is read as a separate message.
func TestServeOverStdioReadsALongMessageWholeAndTheNextOneAfterIt(t *testing.T) {
	responder, _ := buildTestResponder(func(map[string]any) (any, error) {
		return map[string]any{"ran": true}, nil
	})
	padded := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` +
		strings.Repeat("x", 100_000) + `"}}`
	input := padded + "\n" + `{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n"

	answers := []string{}
	ServeOverStdio(context.Background(), responder, strings.NewReader(input),
		func(line string) { answers = append(answers, line) })

	if len(answers) != 2 {
		t.Fatalf("the server wrote %v", answers)
	}
	if answers[0] != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Errorf("the long message answered %s", answers[0])
	}
	if answers[1] != `{"jsonrpc":"2.0","id":2,"result":{}}` {
		t.Errorf("the message after it answered %s", answers[1])
	}
}

func TestTheQuestionIsAskedAndAnswered(t *testing.T) {
	asked := make(chan map[string]any, 1)
	asker := CreateAsker(func(string) {})
	asker.RememberClient(map[string]any{"elicitation": map[string]any{}})
	asker.AttachWriter(func(message any) {
		written, _ := json.Marshal(message)
		read := map[string]any{}
		_ = json.Unmarshal(written, &read)
		asked <- read
	})

	said := make(chan bool, 1)
	go func() { said <- asker.AskConfirmation(context.Background(), "confirm on one", "a body") }()

	question := <-asked
	if question["method"] != "elicitation/create" {
		t.Fatalf("the server sent %v", question)
	}
	params, _ := question["params"].(map[string]any)
	if params["message"] != "confirm on one\n\na body" {
		t.Errorf("the question reads %v", params["message"])
	}
	if !asker.ReceiveAnswer(map[string]any{
		"id":     question["id"],
		"result": map[string]any{"action": "accept", "content": map[string]any{"confirm": true}},
	}) {
		t.Fatal("the answer was not taken")
	}
	if !<-said {
		t.Error("the answer of the user does not read as a yes")
	}
}

func TestTheQuestionIsNotAskedOfAClientThatCannot(t *testing.T) {
	asker := CreateAsker(func(string) {})
	asker.AttachWriter(func(any) { t.Error("a client that cannot ask was asked") })
	if asker.CanAsk() {
		t.Error("a client that said nothing reads as able to ask")
	}
	if asker.AskConfirmation(context.Background(), "confirm", "a body") {
		t.Error("a client that cannot ask answered yes")
	}
}

func TestAQuestionThatIsNotAnsweredInTimeIsANo(t *testing.T) {
	written := []string{}
	asker := CreateAsker(func(message string) { written = append(written, message) })
	asker.answerTimeout = 10 * time.Millisecond
	asker.RememberClient(map[string]any{"elicitation": map[string]any{}})
	asker.AttachWriter(func(any) {})

	if asker.AskConfirmation(context.Background(), "confirm", "a body") {
		t.Error("a question nobody answered reads as a yes")
	}
	if len(written) != 3 || !strings.Contains(written[1], "was not answered in time") {
		t.Errorf("the log holds %v", written)
	}
}

func TestReadsAsYes(t *testing.T) {
	cases := []struct {
		result any
		yes    bool
	}{
		{map[string]any{"action": "accept",
			"content": map[string]any{"confirm": true}}, true},
		{map[string]any{"action": "accept",
			"content": map[string]any{"confirm": false}}, false},
		{map[string]any{"action": "decline"}, false},
		{map[string]any{"action": "accept"}, false},
		{nil, false},
		{"accept", false},
	}
	for _, held := range cases {
		if answered := readsAsYes(held.result); answered != held.yes {
			t.Errorf("%v reads as %v, wanted %v", held.result, answered, held.yes)
		}
	}
}

func TestReadIncomingMessage(t *testing.T) {
	cases := []struct {
		written     string
		kind        int
		wantsAnswer bool
		method      string
	}{
		{`{"id":1,"method":"ping"}`, messageCall, true, "ping"},
		{`{"method":"ping"}`, messageCall, false, "ping"},
		{`{"id":null,"method":"ping"}`, messageCall, false, "ping"},
		{`{"id":"a"}`, messageMethodless, true, ""},
		{`{}`, messageMethodless, false, ""},
		{`"text"`, messageMalformed, false, ""},
	}
	for _, held := range cases {
		var message any
		if err := json.Unmarshal([]byte(held.written), &message); err != nil {
			t.Fatalf("%q is not JSON", held.written)
		}
		read := readIncomingMessage(message)
		if read.kind != held.kind || read.wantsAnswer != held.wantsAnswer ||
			read.method != held.method {
			t.Errorf("%s read as kind %d, answer %v, method %q",
				held.written, read.kind, read.wantsAnswer, read.method)
		}
	}
}
