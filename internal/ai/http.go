package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// What both providers share: how a failure reads, how a request is sent, how the stream of
// events that returns it is read, and how one call of the model is built out of it.

// maxReportedFailure is how much of the body of a failure is reported, in characters.
const maxReportedFailure = 400

// maxReadFailureBytes is how much of the body is read to find those characters, because UTF-8
// writes one character as up to four bytes.
const maxReadFailureBytes = maxReportedFailure * 4

// streamingClient is the one HTTP client both providers send through. It carries no deadline
// of its own, because a reply arrives as a stream and a long one must not be cut in the
// middle. What can hang before the first byte is limited by the transport instead.
var streamingClient = &http.Client{Transport: buildTransport()}

// sendJSON sends one request and returns its body, which the caller reads as a stream.
func sendJSON(
	ctx context.Context, url string, headers map[string]string, body any,
) (io.ReadCloser, error) {
	written, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	asked, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		strings.NewReader(string(written)))
	if err != nil {
		return nil, err
	}
	asked.Header.Set("content-type", "application/json")
	for name, value := range headers {
		asked.Header.Set(name, value)
	}

	answered, err := streamingClient.Do(asked)
	if err != nil {
		return nil, err
	}
	if answered.StatusCode >= 300 {
		defer func() { _ = answered.Body.Close() }()
		return nil, fmt.Errorf("%s", describeFailedRequest(answered))
	}
	return answered.Body, nil
}

// describeFailedRequest writes what a provider said when it refused the request.
func describeFailedRequest(answered *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(answered.Body, maxReadFailureBytes))
	if message := findFailureMessage(body); message != "" {
		return message
	}
	written := strings.TrimSpace(string(body))
	if written == "" {
		return answered.Status
	}
	if len([]rune(written)) > maxReportedFailure {
		written = string([]rune(written)[:maxReportedFailure]) + "…"
	}
	return answered.Status + ": " + written
}

// findFailureMessage reads the message out of the answer of a provider. Both write it under
// `error`, one as an object and one as a text.
func findFailureMessage(body []byte) string {
	read := map[string]any{}
	if err := json.Unmarshal(body, &read); err != nil {
		return ""
	}
	switch held := read["error"].(type) {
	case string:
		return held
	case map[string]any:
		if message, is := held["message"].(string); is {
			return message
		}
	}
	return ""
}

// serverEvent is one event of a stream, as the two providers both write them.
type serverEvent struct {
	data string
}

// readServerEvents reads the events of one stream and hands each one over. It ends when the
// stream ends, or when the reader returns a failure.
func readServerEvents(body io.Reader, onEvent func(serverEvent) error) error {
	reader := bufio.NewReaderSize(body, 64*1024)
	held := serverEvent{}

	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")

		switch {
		case trimmed == "":
			// A blank line closes one event.
			if held.data != "" {
				if problem := onEvent(held); problem != nil {
					return problem
				}
			}
			held = serverEvent{}
		case strings.HasPrefix(trimmed, "data:"):
			// One space after the colon belongs to the protocol, and every space after
			// that belongs to the data.
			part := strings.TrimPrefix(trimmed[len("data:"):], " ")
			if held.data == "" {
				held.data = part
			} else {
				held.data += "\n" + part
			}
		}

		if err != nil {
			if err == io.EOF {
				if held.data != "" {
					return onEvent(held)
				}
				return nil
			}
			return err
		}
	}
}

// readJSONInto reads the data of one event as the value a provider named.
func readJSONInto(data string, into any) error {
	return json.Unmarshal([]byte(data), into)
}

// openBlock is the block of the stream being read, which arrives a piece at a time. Both
// protocols write a call this way, an argument at a time under a name that came first.
type openBlock struct {
	kind      string
	callID    string
	name      string
	arguments strings.Builder
}

// readToolCall reads the input a model wrote for one call. An input that cannot be read is
// carried as it came, so the tool reports what is wrong with it.
func readToolCall(id, name, arguments string) ToolCall {
	call := ToolCall{ID: id, Name: name, Arguments: arguments, Input: map[string]any{}}
	if strings.TrimSpace(arguments) == "" {
		call.Arguments = "{}"
		return call
	}
	read := map[string]any{}
	if err := json.Unmarshal([]byte(arguments), &read); err == nil {
		call.Input = read
	}
	return call
}
