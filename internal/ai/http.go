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

// The parts both providers share: the form of an error, the way a request is sent, the way
// the event stream of the answer is read, and the way one tool call is built from it.

// maxReportedFailure is the number of characters of an error body that are reported.
const maxReportedFailure = 400

// maxReadFailureBytes is the number of bytes read to get those characters, because UTF-8
// uses up to four bytes per character.
const maxReadFailureBytes = maxReportedFailure * 4

// streamingClient is the HTTP client of both providers. It has no time limit of its own,
// because a reply arrives as a stream and a long reply must not be cut. The transport limits
// the time before the first byte.
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

// describeFailedRequest returns the error message of a provider that refused the request.
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

// findFailureMessage reads the message from the answer of a provider. Both providers use the
// key `error`, one with an object and one with a string.
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

// serverEvent is one event of a stream, in the form both providers use.
type serverEvent struct {
	data string
}

// readServerEvents reads the events of one stream and passes each one to the reader. It
// returns at the end of the stream, or when the reader returns an error.
func readServerEvents(body io.Reader, onEvent func(serverEvent) error) error {
	reader := bufio.NewReaderSize(body, 64*1024)
	held := serverEvent{}

	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")

		switch {
		case trimmed == "":
			// A blank line ends one event.
			if held.data != "" {
				if problem := onEvent(held); problem != nil {
					return problem
				}
			}
			held = serverEvent{}
		case strings.HasPrefix(trimmed, "data:"):
			// One space after the colon belongs to the protocol. Every further space
			// belongs to the data.
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

// readJSONInto parses the data of one event into the value of the provider.
func readJSONInto(data string, into any) error {
	return json.Unmarshal([]byte(data), into)
}

// openBlock is the block of the stream in progress. It arrives in parts. Both protocols send
// a call this way: the name first, then the arguments in parts.
type openBlock struct {
	kind      string
	callID    string
	name      string
	arguments strings.Builder
}

// readToolCall parses the input the model wrote for one call. An input that cannot be parsed
// is passed on unchanged, so the tool reports the problem.
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
