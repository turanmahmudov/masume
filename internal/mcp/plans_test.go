package mcp

import (
	"os"
	"testing"
	"time"
)

// TestMain keeps the log of the tests out of the state directory of the user. The path is
// read one time per process, so it is set before any test runs.
func TestMain(suite *testing.M) {
	held, err := os.MkdirTemp("", "masume-mcp-test")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_STATE_HOME", held); err != nil {
		panic(err)
	}
	code := suite.Run()
	_ = os.RemoveAll(held)
	os.Exit(code)
}

// The rows move on, so a plan the user read an hour ago says nothing about them now.
func TestATokenGoesStale(t *testing.T) {
	tokens := CreatePlanTokens()
	written := "delete from orders where id = 1"
	token := tokens.Issue("shop", written)

	tokens.now = func() time.Time { return time.Now().Add(time.Hour) }
	if tokens.Take(token, "shop", written) {
		t.Error("a stale token ran the write")
	}
}
