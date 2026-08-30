package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// The only question the server asks. The protocol calls it elicitation: the server sends a
// request the other way, and the client shows it to the user. A client that does not say it
// can ask is never asked.

// defaultAnswerTimeout is how long the user has to answer before the statement is dropped.
const defaultAnswerTimeout = 120 * time.Second

// Asker asks the user through the client of the agent, and reports the answer.
type Asker struct {
	logEvent func(message string)
	// answerTimeout is how long one question waits.
	answerTimeout time.Duration

	guard sync.Mutex
	// waiting holds one channel per question the client has not answered.
	waiting      map[string]chan any
	write        func(message any)
	clientCanAsk bool
	nextID       int
}

// CreateAsker builds the asker of one server.
func CreateAsker(logEvent func(message string)) *Asker {
	return &Asker{
		logEvent:      logEvent,
		answerTimeout: defaultAnswerTimeout,
		waiting:       map[string]chan any{},
	}
}

// CanAsk is true where the client said at initialize that it can ask its user.
func (asker *Asker) CanAsk() bool {
	asker.guard.Lock()
	defer asker.guard.Unlock()
	return asker.findAskWriter() != nil
}

// findAskWriter returns the writer to ask through, or nothing while the client cannot be
// asked. The caller holds the lock.
func (asker *Asker) findAskWriter() func(message any) {
	if !asker.clientCanAsk {
		return nil
	}
	return asker.write
}

// RememberClient reads the capabilities of the client from the initialize parameters.
func (asker *Asker) RememberClient(capabilities any) {
	named, _ := capabilities.(map[string]any)
	elicitation, is := named["elicitation"].(map[string]any)

	asker.guard.Lock()
	defer asker.guard.Unlock()
	asker.clientCanAsk = is && elicitation != nil
}

// AttachWriter takes the write function, once the transport has one.
func (asker *Asker) AttachWriter(write func(message any)) {
	asker.guard.Lock()
	defer asker.guard.Unlock()
	asker.write = write
}

// ReceiveAnswer is true where the message answered a question this asker sent.
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

// AskConfirmation is true only where the user saw the question and agreed.
func (asker *Asker) AskConfirmation(ctx context.Context, title, body string) bool {
	asker.guard.Lock()
	write := asker.findAskWriter()
	if write == nil {
		asker.guard.Unlock()
		return false
	}
	asker.nextID++
	id := fmt.Sprintf("ask-%d", asker.nextID)
	// Buffered, so the answer of a question that ran out of time is not held by anybody.
	answered := make(chan any, 1)
	asker.waiting[id] = answered
	asker.guard.Unlock()

	asker.logEvent("> ask " + id + " " + title)
	write(requestMessage{
		JSONRPC: jsonRPCVersion, ID: id, Method: "elicitation/create",
		Params: map[string]any{
			"message":         title + "\n\n" + body,
			"requestedSchema": buildAnswerSchema(body),
		},
	})

	// The answer of the user is a message on the stream this call was read from.
	releaseReader(ctx)
	said := readsAsYes(asker.waitForAnswer(id, answered))
	if said {
		asker.logEvent("< ask " + id + " yes")
	} else {
		asker.logEvent("< ask " + id + " no")
	}
	return said
}

// waitForAnswer returns what the client said, and nothing where it said nothing in time. A
// client that gives no answer leaves the statement unrun, so the server does not wait forever.
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
			// The answer arrived as the time ran out.
			return <-answered
		}
		asker.logEvent("! " + id + " was not answered in time")
		return nil
	}
}

// buildAnswerSchema writes the schema of the answer: one field the user confirms.
func buildAnswerSchema(body string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirm": map[string]any{
				"type": "boolean", "title": "Run this statement", "description": body,
			},
		},
		"required": []string{"confirm"},
	}
}

// readsAsYes is true only for an answer the user gave, and only for a yes.
func readsAsYes(result any) bool {
	answer, is := result.(map[string]any)
	if !is || answer["action"] != "accept" {
		return false
	}
	content, is := answer["content"].(map[string]any)
	return is && content["confirm"] == true
}
