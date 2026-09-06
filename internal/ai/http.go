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

// Shared HTTP requests, provider errors, event streams, and tool argument parsing.

// maxReportedFailure is the number of characters of an error body that are reported.
const maxReportedFailure = 400

// maxReadFailureBytes is the error body byte limit. UTF-8 uses up to four bytes per character.
const maxReadFailureBytes = maxReportedFailure * 4

// streamingClient is the shared HTTP client. Transport timeouts apply to connection setup and response headers, without a total request timeout.
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

// findFailureMessage accepts provider errors as strings or objects with a message field.
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

// readServerEvents reads server-sent events until EOF or an error.
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
			// SSE permits one optional space after the colon. Additional spaces are data.
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

// openBlock is a tool call assembled from streamed argument fragments.
type openBlock struct {
	kind      string
	callID    string
	name      string
	arguments strings.Builder
}

// readToolCall preserves raw arguments. Invalid JSON leaves Input empty.
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
