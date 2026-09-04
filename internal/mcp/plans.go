package mcp

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// The tokens of a write plan. A client that cannot show a question of its own leaves the
// agent to ask the user in its own words. The token is what the agent brings back: it says
// that this one statement, on this one connection, was measured and shown.

// planTokenLife is how long a token is worth anything. It is short: the user answered a
// plan that was measured then, and the rows move on.
const planTokenLife = 10 * time.Minute

// issuedPlan is one token and the write it was issued for.
type issuedPlan struct {
	profile string
	sql     string
	at      time.Time
}

// PlanTokens holds the tokens issued in this session.
type PlanTokens struct {
	guard  sync.Mutex
	issued map[string]issuedPlan
	next   int
	// now is the clock, so a test can move it.
	now func() time.Time
}

// CreatePlanTokens returns an empty store.
func CreatePlanTokens() *PlanTokens {
	return &PlanTokens{issued: map[string]issuedPlan{}, now: time.Now}
}

// Issue returns a token for one write on one connection.
func (tokens *PlanTokens) Issue(profile, sql string) string {
	if tokens == nil {
		return ""
	}
	tokens.guard.Lock()
	defer tokens.guard.Unlock()

	tokens.next++
	token := fmt.Sprintf("plan-%d", tokens.next)
	tokens.issued[token] = issuedPlan{profile: profile, sql: sql, at: tokens.now()}
	return token
}

// Take is true where the token was issued for this write on this connection and has not
// been used. Every token is taken one time, so an agent cannot run one plan twice.
func (tokens *PlanTokens) Take(token, profile, sql string) bool {
	if tokens == nil || token == "" {
		return false
	}
	tokens.guard.Lock()
	defer tokens.guard.Unlock()

	held, issued := tokens.issued[token]
	if !issued {
		return false
	}
	delete(tokens.issued, token)
	if held.profile != profile || !matchesStatement(held.sql, sql) {
		return false
	}
	return tokens.now().Sub(held.at) <= planTokenLife
}

// matchesStatement is true for the same statement, whatever the spacing around it. The
// statement decides what runs, so nothing else about it is allowed to differ.
func matchesStatement(issued, asked string) bool {
	return strings.TrimSpace(issued) == strings.TrimSpace(asked)
}
