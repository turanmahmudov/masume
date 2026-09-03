package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// The transport of the protocol: one message per line, over standard input and standard
// output. It reads a message, passes it to the responder, and writes the answer.

// releaseKey is the context key of the function that lets the transport read the next
// message while this call waits.
type releaseKey struct{}

// releaseReader lets the server read the next message while this call waits for the user,
// because the answer of the user arrives on the same stream.
func releaseReader(ctx context.Context) {
	if release, is := ctx.Value(releaseKey{}).(func()); is {
		release()
	}
}

// The two limits on the load one client can put on this server. Both apply to any input: a
// message without a line break would grow the buffer until the machine has no memory left,
// and a client that never waits for an answer would start a goroutine, and a connection, for
// every line.
const (
	// maxMessageBytes is the maximum length of a message. A call holds a statement, and
	// no statement a person writes is near this size.
	maxMessageBytes = 1 << 20
	// maxCallsAtOnce is the number of calls that can run in parallel. A call that
	// reached a server released the reader, so without this limit the number would be
	// the number of lines the client sent. It is a safety limit, far above the load of a
	// real client.
	maxCallsAtOnce = 64
)

// ServeOverStdio runs until the client closes the stream. It answers one message before it
// reads the next one, so the answers keep the order of the calls. A call that asks the user
// releases the reader first.
func ServeOverStdio(
	ctx context.Context, responder *Responder, input io.Reader, write func(line string),
) {
	reader := bufio.NewReader(input)
	answering := sync.WaitGroup{}
	// Every answer is written before the server stops, because every accepted call gets
	// an answer.
	defer answering.Wait()
	// Capacity for the calls that released the reader and still run.
	room := make(chan struct{}, maxCallsAtOnce)

	for {
		line, err := readMessageLine(reader)
		if errors.Is(err, errMessageTooLong) {
			write(buildJSONLine(buildError(nil, invalidRequest, fmt.Sprintf(
				"a message may be at most %d bytes", maxMessageBytes))))
			return
		}
		if strings.TrimSpace(line) != "" {
			answerOneLine(ctx, responder, &answering, room, line, write)
		}
		if err != nil {
			return
		}
	}
}

// errMessageTooLong is returned for a line above the limit. The rest of that line cannot be
// separated from the next message, so the server stops and does not answer a part of it.
var errMessageTooLong = errors.New("the message is longer than the server reads")

// readMessageLine reads one message, up to the limit. It reads one buffer at a time, because
// a reader that asks for a whole line grows to the size the client sends before the first
// line break, and the client would then control the memory of this process.
func readMessageLine(reader *bufio.Reader) (string, error) {
	var held strings.Builder
	for {
		part, err := reader.ReadSlice('\n')
		if held.Len()+len(part) > maxMessageBytes {
			return "", errMessageTooLong
		}
		held.Write(part)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return strings.TrimSuffix(held.String(), "\n"), err
	}
}

// answerOneLine parses one line as a message and answers it. It returns after the answer is
// written, or after the call starts to wait for the user.
func answerOneLine(
	ctx context.Context, responder *Responder, answering *sync.WaitGroup,
	room chan struct{}, line string, write func(line string),
) {
	var message any
	if err := json.Unmarshal([]byte(line), &message); err != nil {
		write(buildJSONLine(buildError(nil, parseError, "the message is not valid JSON")))
		return
	}

	// A call takes a slot before it starts. A message that is not a call takes none. The
	// answer of the user to a question of the server is such a message, and the calls
	// that wait for it fill the slots, so a slot for the answer would block them for
	// ever.
	held := readIncomingMessage(message)
	takesRoom := held.kind == messageCall
	if takesRoom {
		select {
		case room <- struct{}{}:
		default:
			write(buildJSONLine(buildError(held.id, internalError, fmt.Sprintf(
				"%d calls are already running; wait for one to answer", maxCallsAtOnce))))
			return
		}
	}

	released := make(chan struct{})
	release := sync.OnceFunc(func() { close(released) })
	answering.Go(func() {
		if takesRoom {
			defer func() { <-room }()
		}
		defer release()
		answer := responder.AnswerMessage(context.WithValue(ctx, releaseKey{}, release), message)
		if answer != nil {
			write(buildJSONLine(answer))
		}
	})
	<-released
}
