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
// output. It reads, hands each message to the responder, and writes what comes back.

// releaseKey names the function that lets the transport read the next message while this call
// waits.
type releaseKey struct{}

// releaseReader lets the server read the next message while this call waits for the user,
// because the answer of the user arrives on the same stream.
func releaseReader(ctx context.Context) {
	if release, is := ctx.Value(releaseKey{}).(func()); is {
		release()
	}
}

// The two limits on what one client can ask of this server. Both hold whatever the client
// sends: a message with no line break would otherwise grow the buffer until the machine has
// no memory left, and a client that never waits for an answer would start a goroutine, and
// a connection behind it, for every line.
const (
	// maxMessageBytes is the longest message this server reads. A call carries a
	// statement, and no statement a person writes comes near this.
	maxMessageBytes = 1 << 20
	// maxCallsAtOnce is how many calls may run beside each other. A call that reached a
	// server has released the reader, so without this the count would be the count of
	// the lines the client sent. It is a backstop, well above any real client.
	maxCallsAtOnce = 64
)

// ServeOverStdio runs until the client closes its end. One message is answered before the next
// is read, so the answers keep the order of the calls, and a call that asks the user releases
// the reader first.
func ServeOverStdio(
	ctx context.Context, responder *Responder, input io.Reader, write func(line string),
) {
	reader := bufio.NewReader(input)
	answering := sync.WaitGroup{}
	// Every answer is written before the server ends, because a call that was taken is
	// answered.
	defer answering.Wait()
	// Room for the calls that released the reader and are still running.
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
// told from the message after it, so the server stops rather than answer a part of it.
var errMessageTooLong = errors.New("the message is longer than the server reads")

// readMessageLine reads one message, up to the limit. It reads a buffer at a time, because
// a reader asked for a whole line grows to hold whatever a client sends before the first
// line break, and that is the client saying how much memory this process takes.
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

// answerOneLine reads one line as a message and returns it, and comes back once the answer is
// written or the call began to wait for the user.
func answerOneLine(
	ctx context.Context, responder *Responder, answering *sync.WaitGroup,
	room chan struct{}, line string, write func(line string),
) {
	var message any
	if err := json.Unmarshal([]byte(line), &message); err != nil {
		write(buildJSONLine(buildError(nil, parseError, "the message is not valid JSON")))
		return
	}

	// A call takes a place before it starts. A message that is no call takes none: the
	// answer of the user to a question the server asked is one of those, and the calls
	// waiting for it are what fills the room, so holding it back would leave them waiting
	// for ever.
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
