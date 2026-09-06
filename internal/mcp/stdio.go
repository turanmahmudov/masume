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

// The stdio transport reads and writes one JSON-RPC message per line.

// releaseKey is the context key for the reader release function.
type releaseKey struct{}

// releaseReader permits further input while a call waits. Confirmation responses arrive on the same stream.
func releaseReader(ctx context.Context) {
	if release, is := ctx.Value(releaseKey{}).(func()); is {
		release()
	}
}

// Per-client input and concurrency limits.
const (
	// maxMessageBytes is the maximum message length.
	maxMessageBytes = 1 << 20
	// maxCallsAtOnce is the maximum concurrent calls.
	maxCallsAtOnce = 64
)

// ServeOverStdio reads until EOF or an input error. Calls can release the reader before completion.
func ServeOverStdio(
	ctx context.Context, responder *Responder, input io.Reader, write func(line string),
) {
	reader := bufio.NewReader(input)
	answering := sync.WaitGroup{}
	// Wait for accepted calls before returning.
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

// errMessageTooLong is an input line above the message limit.
var errMessageTooLong = errors.New("the message exceeds the server size limit")

// readMessageLine reads a line in bounded fragments and checks the message limit before appending.
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

// answerOneLine starts message processing and waits until the call releases the reader.
func answerOneLine(
	ctx context.Context, responder *Responder, answering *sync.WaitGroup,
	room chan struct{}, line string, write func(line string),
) {
	var message any
	if err := json.Unmarshal([]byte(line), &message); err != nil {
		write(buildJSONLine(buildError(nil, parseError, "the message is not valid JSON")))
		return
	}

	// Confirmation responses bypass call slots. Waiting calls may occupy every slot.
	held := readIncomingMessage(message)
	takesRoom := held.kind == messageCall
	if takesRoom {
		select {
		case room <- struct{}{}:
		default:
			write(buildJSONLine(buildError(held.id, internalError, fmt.Sprintf(
				"%d calls are already running; wait for a call to finish", maxCallsAtOnce))))
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
