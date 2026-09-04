package session

import (
	"fmt"
	"io"
	"os"
)

// Interact passes control of the active session to the user's interactive terminal,
// similar to pexpect.interact(). It enables remote terminal echo so commands are displayed,
// places the local terminal into raw mode, and forwards all keystrokes and outputs until the child process exits.
func (s *Session) Interact() error {
	return s.InteractWithIO(os.Stdin, os.Stdout)
}

// InteractWithIO passes control of the active session using the specified input and output streams.
func (s *Session) InteractWithIO(in io.Reader, out io.Writer) error {
	if s.child == nil {
		return fmt.Errorf("interact: no active session process")
	}

	// Restore remote echo so the interactive shell displays characters
	_ = s.child.Send("stty echo\n")

	// Ensure output writer is unmuted
	s.logger.outputWriter.Unmute()

	// Hand over terminal control to the interactive user
	err := s.child.InteractWithIO(in, out)

	// When returning, restore echo off if the session is still running
	if s.child.IsAlive() {
		_ = s.child.Send("stty -echo\n")
	}
	return err
}
