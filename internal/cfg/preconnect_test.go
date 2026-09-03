package cfg

import (
	"testing"
	"time"
)

func TestStopWaitsForTheCommandItStopped(t *testing.T) {
	handle, err := StartPreConnectCommand(Profile{Name: "tunnel", Command: "sleep 30"})
	if err != nil {
		t.Fatalf("the command did not start: %v", err)
	}
	command := handle.command

	handle.Stop()

	// A command that got a signal but no wait keeps an entry in the process table.
	if command.ProcessState == nil {
		t.Error("the command was stopped and never waited for")
	}
}

func TestStartPreConnectCommandGivesUpWhenTheCommandLeaves(t *testing.T) {
	profile := Profile{
		Name: "tunnel", Command: "exit 1", Host: "127.0.0.1",
		WaitForPort: 59_999, CommandTimeout: 20 * time.Second,
	}

	started := time.Now()
	handle, err := StartPreConnectCommand(profile)
	elapsed := time.Since(started)

	if err == nil {
		handle.Stop()
		t.Fatal("a command that left answered a handle")
	}
	if elapsed > 10*time.Second {
		t.Errorf("the port was tried for %s after the command left", elapsed)
	}
}
