package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MCP elicitation sends confirmation requests only to clients that report elicitation support.

// defaultAnswerTimeout is the confirmation timeout before execution is refused.
const defaultAnswerTimeout = 120 * time.Second

// Asker requests user confirmation through the MCP client.
type Asker struct {
	logEvent func(message string)
	// answerTimeout is the time one question waits for an answer.
	answerTimeout time.Duration

	guard sync.Mutex
	// waiting holds one channel per open question.
	waiting      map[string]chan any
	write        func(message any)
	clientCanAsk bool
	nextID       int
}

// CreateAsker returns the asker of one server.
func CreateAsker(logEvent func(message string)) *Asker {
	return &Asker{
		logEvent:      logEvent,
		answerTimeout: defaultAnswerTimeout,
		waiting:       map[string]chan any{},
	}
}

// CanAsk is true if the client reported at initialize that it can ask its user.
func (asker *Asker) CanAsk() bool {
	asker.guard.Lock()
	defer asker.guard.Unlock()
	return asker.findAskWriter() != nil
}

// findAskWriter returns the confirmation writer or nil. The caller holds the lock.
func (asker *Asker) findAskWriter() func(message any) {
	if !asker.clientCanAsk {
		return nil
	}
	return asker.write
}

// RememberClient records elicitation support from the initialize capabilities.
func (asker *Asker) RememberClient(capabilities any) bool {
	named, _ := capabilities.(map[string]any)
	elicitation, is := named["elicitation"].(map[string]any)

	asker.guard.Lock()
	defer asker.guard.Unlock()
	asker.clientCanAsk = is && elicitation != nil
	return asker.clientCanAsk
}

// AttachWriter stores the write function after the transport has one.
func (asker *Asker) AttachWriter(write func(message any)) {
	asker.guard.Lock()
	defer asker.guard.Unlock()
	asker.write = write
}

// ReceiveAnswer is true if the message is the answer to a question of this asker.
func (asker *Asker) ReceiveAnswer(message map[string]any) bool {
	id, is := message["id"].(string)
	if !is {
		return false
	}

	asker.guard.Lock()
	defer asker.guard.Unlock()
	answered, waiting := asker.waiting[id]
	if !waiting {
		return false
	}
	delete(asker.waiting, id)
	answered <- message["result"]
	return true
}

// AskConfirmation is true only if the user saw the question and confirmed.
func (asker *Asker) AskConfirmation(ctx context.Context, title, body string) bool {
	asker.guard.Lock()
	write := asker.findAskWriter()
	if write == nil {
		asker.guard.Unlock()
		return false
	}
	asker.nextID++
	id := fmt.Sprintf("ask-%d", asker.nextID)
	// The buffered channel accepts late responses without blocking.
	answered := make(chan any, 1)
	asker.waiting[id] = answered
	asker.guard.Unlock()

	asker.logEvent("> ask " + id + " " + title)
	write(requestMessage{
		JSONRPC: jsonRPCVersion, ID: id, Method: "elicitation/create",
		Params: map[string]any{
			"message":         title + "\n\n" + body,
			"requestedSchema": buildAnswerSchema(),
		},
	})

	// The answer of the user arrives on the same stream as this call.
	releaseReader(ctx)
	said := readsAsYes(asker.waitForAnswer(id, answered))
	if said {
		asker.logEvent("< ask " + id + " yes")
	} else {
		asker.logEvent("< ask " + id + " no")
	}
	return said
}

// waitForAnswer returns the client response or nil after timeout.
func (asker *Asker) waitForAnswer(id string, answered chan any) any {
	timer := time.NewTimer(asker.answerTimeout)
	defer timer.Stop()

	select {
	case result := <-answered:
		return result
	case <-timer.C:
		asker.guard.Lock()
		_, pending := asker.waiting[id]
		delete(asker.waiting, id)
		asker.guard.Unlock()
		if !pending {
			// The answer arrived at the same time as the timeout.
			return <-answered
		}
		asker.logEvent("! " + id + " was not answered in time")
		return nil
	}
}

// buildAnswerSchema returns a schema with one required confirmation field.
func buildAnswerSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirm": map[string]any{
				"type": "boolean", "title": "Run this statement",
				"description": "Yes permits execution of the statement above. No cancels execution.",
			},
		},
		"required": []string{"confirm"},
	}
}

// readsAsYes is true only for an answer of the user, and only for a confirmation.
func readsAsYes(result any) bool {
	answer, is := result.(map[string]any)
	if !is || answer["action"] != "accept" {
		return false
	}
	content, is := answer["content"].(map[string]any)
	return is && content["confirm"] == true
}
