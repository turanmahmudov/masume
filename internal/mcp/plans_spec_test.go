package mcp_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/mcp"
)

func TestATokenRunsTheWriteItWasIssuedFor(t *testing.T) {
	tokens := mcp.CreatePlanTokens()
	written := "delete from orders where id = 1"
	token := tokens.Issue("shop", written)

	if !tokens.Take(token, "shop", written) {
		t.Error("the token of this write was refused")
	}
}

// Whitespace around the statement is not the statement, so a plan measured with a trailing
// newline still runs.
func TestATokenIgnoresTheSpaceAroundTheStatement(t *testing.T) {
	tokens := mcp.CreatePlanTokens()
	token := tokens.Issue("shop", "delete from orders where id = 1")

	if !tokens.Take(token, "shop", "  delete from orders where id = 1\n") {
		t.Error("the same statement was refused")
	}
}

// The token stands for one statement on one connection. Anything else was not what the user
// read, so it does not run.
func TestATokenRunsNothingElse(t *testing.T) {
	written := "delete from orders where id = 1"
	for _, refused := range []struct {
		name    string
		profile string
		sql     string
	}{
		{"another statement", "shop", "delete from orders"},
		{"another connection", "shop-prod", written},
		{"a statement with one value changed", "shop", "delete from orders where id = 2"},
	} {
		tokens := mcp.CreatePlanTokens()
		token := tokens.Issue("shop", written)
		if tokens.Take(token, refused.profile, refused.sql) {
			t.Errorf("the token ran %s", refused.name)
		}
	}
}

func TestATokenRunsOneWriteOnly(t *testing.T) {
	tokens := mcp.CreatePlanTokens()
	written := "delete from orders where id = 1"
	token := tokens.Issue("shop", written)

	tokens.Take(token, "shop", written)
	if tokens.Take(token, "shop", written) {
		t.Error("the token ran the write a second time")
	}
}

func TestAnUnknownTokenRunsNothing(t *testing.T) {
	tokens := mcp.CreatePlanTokens()
	tokens.Issue("shop", "delete from orders where id = 1")

	for _, token := range []string{"", "plan-9", "plan-1 "} {
		if tokens.Take(token, "shop", "delete from orders where id = 1") {
			t.Errorf("the write ran on token %q", token)
		}
	}
}
