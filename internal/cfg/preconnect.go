package cfg

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"time"
)

// A pre-connect command starts a tunnel or proxy before the database connection opens.

// pollInterval is the time between two tests of the port while the command starts.
const pollInterval = 100 * time.Millisecond

// portDialTimeout is the timeout of one test of the port.
const portDialTimeout = time.Second

// stopGrace is the time the process group has to stop after SIGTERM before it is killed.
const stopGrace = 2 * time.Second

// PreConnectHandle is a running pre-connect command and the data needed to stop it.
type PreConnectHandle struct {
	command *exec.Cmd
	// exited closes after Wait. The process ID remains reserved until Wait returns.
	exited chan struct{}
}

// Stop stops the command and its process group, including child processes such as ssh.
func (handle *PreConnectHandle) Stop() {
	if handle == nil || handle.command == nil || handle.command.Process == nil {
		return
	}
	command := handle.command
	handle.command = nil

	select {
	case <-handle.exited:
		return
	default:
	}

	if syscall.Kill(-command.Process.Pid, syscall.SIGTERM) != nil {
		_ = command.Process.Kill()
	}
	select {
	case <-handle.exited:
	case <-time.After(stopGrace):
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		<-handle.exited
	}
}

// isPortOpen is true if a process accepts a connection on the port.
func isPortOpen(host string, port int) bool {
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	connection, err := net.DialTimeout("tcp", address, portDialTimeout)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

// waitForPort waits for a TCP listener, command exit, or timeout.
func waitForPort(host string, port int, timeout time.Duration, exited <-chan struct{}) bool {
	deadline := time.Now().Add(timeout)
	for {
		if isPortOpen(host, port) {
			return true
		}
		select {
		case <-exited:
			// A command can exit after starting a background listener.
			return isPortOpen(host, port)
		default:
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
}

// StartPreConnectCommand starts the profile command. The caller must stop the handle when the connection closes.
func StartPreConnectCommand(profile Profile) (*PreConnectHandle, error) {
	if profile.Command == "" {
		return &PreConnectHandle{}, nil
	}

	// The command and its children share a separate process group.
	command := exec.Command("sh", "-c", profile.Command)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf(
			"the pre-connect command for %s failed to start: %w", profile.Name, err)
	}
	handle := &PreConnectHandle{command: command, exited: make(chan struct{})}
	go func() {
		_ = command.Wait()
		close(handle.exited)
	}()

	if profile.WaitForPort == 0 {
		return handle, nil
	}
	if waitForPort(profile.Host, profile.WaitForPort, profile.CommandTimeout, handle.exited) {
		return handle, nil
	}

	handle.Stop()
	return nil, fmt.Errorf(
		"the pre-connect command for %s did not open port %d within %.0fs: %s",
		profile.Name, profile.WaitForPort, profile.CommandTimeout.Seconds(), profile.Command)
}
