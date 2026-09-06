package mcp

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Write plan tokens authorize one measured statement on one profile after agent confirmation.

// planTokenLife is the token lifetime.
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
	// now is the clock.
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

// Take consumes a matching token and checks its lifetime. A profile or statement mismatch leaves the token unchanged.
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
	if held.profile != profile || !matchesStatement(held.sql, sql) {
		return false
	}
	delete(tokens.issued, token)
	return tokens.now().Sub(held.at) <= planTokenLife
}

// matchesStatement compares exact statement text after trimming surrounding whitespace.
func matchesStatement(issued, asked string) bool {
	return strings.TrimSpace(issued) == strings.TrimSpace(asked)
}
