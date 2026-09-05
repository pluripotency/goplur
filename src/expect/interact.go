package expect

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/goterm/term"
	xterm "golang.org/x/term"
)

func setWinsize(f *os.File, w, h int) {
	ws := term.Winsize{
		WsRow: uint16(h),
		WsCol: uint16(w),
	}
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
}

// IsAlive returns true if the spawned process is currently running.
func (e *GExpect) IsAlive() bool {
	return e.check()
}

// Output returns the accumulated output in the GExpect buffer.
func (e *GExpect) Output() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.out.String()
}

// Interact passes control of the spawned process to the user's terminal (stdin/stdout),
// similar to pexpect.interact(). Keystrokes are sent to the child process in raw mode,
// and output is forwarded to stdout until the child process terminates.
func (e *GExpect) Interact() error {
	return e.InteractWithIO(os.Stdin, os.Stdout)
}

// InteractWithIO passes control of the spawned process using the specified input and output streams.
func (e *GExpect) InteractWithIO(in io.Reader, out io.Writer) error {
	if !e.check() {
		return errors.New("expect: process is not running")
	}

	// If input is an *os.File attached to a terminal, enable raw mode
	if f, ok := in.(*os.File); ok {
		fd := int(f.Fd())
		if xterm.IsTerminal(fd) {
			oldState, err := xterm.MakeRaw(fd)
			if err == nil {
				defer xterm.Restore(fd, oldState)
			}
			// Synchronize terminal window size with PTY
			if w, h, err := xterm.GetSize(fd); err == nil && e.pty != nil && e.pty.Master != nil {
				setWinsize(e.pty.Master, w, h)
			}

			// Dynamically monitor SIGWINCH to handle window resize during interact
			sigChan := make(chan os.Signal, 1)
			sigDone := make(chan struct{})
			signal.Notify(sigChan, syscall.SIGWINCH)
			defer func() {
				signal.Stop(sigChan)
				close(sigDone)
			}()

			go func() {
				for {
					select {
					case <-sigDone:
						return
					case <-sigChan:
						if w, h, err := xterm.GetSize(fd); err == nil && e.pty != nil && e.pty.Master != nil {
							setWinsize(e.pty.Master, w, h)
						}
					}
				}
			}()
		}
	}

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				nr, err := in.Read(buf)
				if err != nil || nr == 0 {
					return
				}
				if err := e.Send(string(buf[:nr])); err != nil {
					return
				}
			}
		}
	}()

	// Wait for process to exit
	for e.check() {
		time.Sleep(50 * time.Millisecond)
	}

	close(done)
	return nil
}
