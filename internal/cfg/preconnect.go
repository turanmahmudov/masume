package cfg

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"time"
)

// The command a profile needs before the server can be reached, such as an SSH tunnel or a
// cloud proxy. The command runs for the life of the connection.

// pollInterval is how often the port is tried while the command starts.
const pollInterval = 100 * time.Millisecond

// portDialTimeout is how long one attempt at the port waits.
const portDialTimeout = time.Second

// stopGrace is how long the process group has to leave after SIGTERM before it is killed.
const stopGrace = 2 * time.Second

// PreConnectHandle is a running pre-connect command, and how to stop it.
type PreConnectHandle struct {
	command *exec.Cmd
	// exited is closed once the command was waited for. The process holds a slot in the
	// table until then, so nothing else can run under its number.
	exited chan struct{}
}

// Stop stops the command if it still runs. The whole process group is stopped, because the
// shell can leave the work to a child such as ssh.
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

// isPortOpen is true where something returns on the port.
func isPortOpen(host string, port int) bool {
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	connection, err := net.DialTimeout("tcp", address, portDialTimeout)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

// waitForPort waits until something returns on the port. A tunnel or a proxy is ready only
// when it listens, which is later than the start of its command.
func waitForPort(host string, port int, timeout time.Duration, exited <-chan struct{}) bool {
	deadline := time.Now().Add(timeout)
	for {
		if isPortOpen(host, port) {
			return true
		}
		select {
		case <-exited:
			// A command that puts itself in the background leaves as soon as it listens,
			// so the port is tried once more before the wait gives up.
			return isPortOpen(host, port)
		default:
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
}

// StartPreConnectCommand runs the command a profile needs before the server can be reached.
// It returns a handle that must be stopped when the connection closes.
func StartPreConnectCommand(profile Profile) (*PreConnectHandle, error) {
	if profile.Command == "" {
		return &PreConnectHandle{}, nil
	}

	// Its own process group, so all of it can be stopped later.
	command := exec.Command("sh", "-c", profile.Command)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf(
			"the command for %s did not start: %w", profile.Name, err)
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
		"the command for %s did not open port %d within %.0fs: %s",
		profile.Name, profile.WaitForPort, profile.CommandTimeout.Seconds(), profile.Command)
}
