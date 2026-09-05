package session

import (
	"fmt"
	"io"
	"os"
)

// InteractConfig holds configuration options for Session.Interact.
type InteractConfig struct {
	PreCommand  *string
	PostCommand *string
	PreHook     func(s *Session) error
	PostHook    func(s *Session) error
	SyncWinsize bool
}

// InteractOption is a function that modifies InteractConfig.
type InteractOption func(*InteractConfig)

// WithPreCommand sets the shell command to send before interaction starts.
func WithPreCommand(cmd string) InteractOption {
	return func(c *InteractConfig) {
		c.PreCommand = &cmd
	}
}

// WithPostCommand sets the shell command to send after interaction ends.
func WithPostCommand(cmd string) InteractOption {
	return func(c *InteractConfig) {
		c.PostCommand = &cmd
	}
}

// WithoutCommands disables sending any pre/post commands into the terminal stream (useful for virsh console, serial consoles, etc.).
func WithoutCommands() InteractOption {
	empty := ""
	return func(c *InteractConfig) {
		c.PreCommand = &empty
		c.PostCommand = &empty
	}
}

// WithPreHook executes a custom function on the Session before handing over control to the user.
func WithPreHook(fn func(s *Session) error) InteractOption {
	return func(c *InteractConfig) {
		c.PreHook = fn
	}
}

// WithPostHook executes a custom function on the Session after user interaction finishes.
func WithPostHook(fn func(s *Session) error) InteractOption {
	return func(c *InteractConfig) {
		c.PostHook = fn
	}
}

// WithWinsizeSync configures whether to synchronize terminal window size with PTY.
func WithWinsizeSync(enable bool) InteractOption {
	return func(c *InteractConfig) {
		c.SyncWinsize = enable
	}
}

// resolveInteractConfig builds an InteractConfig by combining node defaults with explicit options.
func (s *Session) resolveInteractConfig(opts ...InteractOption) *InteractConfig {
	cfg := &InteractConfig{
		SyncWinsize: true,
	}

	// Read defaults from CurrentNode if available
	if cn := s.CurrentNode(); cn != nil {
		pre := cn.GetInteractPreCommand()
		if pre == "none" {
			pre = ""
		}
		cfg.PreCommand = &pre

		post := cn.GetInteractPostCommand()
		if post == "none" {
			post = ""
		}
		cfg.PostCommand = &post
	}

	// Apply explicit options
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	return cfg
}

// Interact passes control of the active session to the user's interactive terminal,
// similar to pexpect.interact(). It prepares the terminal according to CurrentNode and supplied options,
// places the local terminal into raw mode, and forwards all keystrokes and outputs until the child process exits.
func (s *Session) Interact(opts ...InteractOption) error {
	return s.InteractWithIO(os.Stdin, os.Stdout, opts...)
}

// InteractWithIO passes control of the active session using the specified input and output streams.
func (s *Session) InteractWithIO(in io.Reader, out io.Writer, opts ...InteractOption) error {
	if s.child == nil {
		return fmt.Errorf("interact: no active session process")
	}

	cfg := s.resolveInteractConfig(opts...)

	// Execute PreHook or send PreCommand if specified
	if cfg.PreHook != nil {
		if err := cfg.PreHook(s); err != nil {
			return err
		}
	} else if cfg.PreCommand != nil && *cfg.PreCommand != "" {
		_ = s.child.Send(*cfg.PreCommand + "\n")
	}

	// Ensure output writer is unmuted
	s.logger.outputWriter.Unmute()

	// Hand over terminal control to the interactive user
	err := s.child.InteractWithIO(in, out)

	// Execute PostHook or restore PostCommand when returning
	if s.child.IsAlive() {
		if cfg.PostHook != nil {
			_ = cfg.PostHook(s)
		} else if cfg.PostCommand != nil && *cfg.PostCommand != "" {
			_ = s.child.Send(*cfg.PostCommand + "\n")
		}
	}

	return err
}
