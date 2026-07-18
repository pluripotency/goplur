package goplur

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/goterm/term"
)

// DefaultTimeout is the default Expect timeout.
const DefaultTimeout = 60 * time.Second

const (
	checkDuration     = 50 * time.Millisecond // Default check duration set to 50ms to prevent latency
	defaultBufferSize = 8192
)

// Option represents one Expecter option.
type Option func(*GExpect) Option

// CheckDuration changes the default duration checking for new incoming data.
func CheckDuration(d time.Duration) Option {
	return func(e *GExpect) Option {
		prev := e.chkDuration
		e.chkDuration = d
		return CheckDuration(prev)
	}
}

// Verbose enables/disables verbose logging of matches and sends.
func Verbose(v bool) Option {
	return func(e *GExpect) Option {
		prev := e.verbose
		e.verbose = v
		return Verbose(prev)
	}
}

// VerboseWriter sets an alternate destination for verbose logs.
func VerboseWriter(w io.Writer) Option {
	return func(e *GExpect) Option {
		prev := e.verboseWriter
		e.verboseWriter = w
		return VerboseWriter(prev)
	}
}

// Tee duplicates all of the spawned process's output to the given writer and
// closes the writer when complete.
func Tee(w io.WriteCloser) Option {
	return func(e *GExpect) Option {
		prev := e.teeWriter
		e.teeWriter = w
		return Tee(prev)
	}
}

// TimeoutError is the error returned by all Expect functions upon timer expiry.
type TimeoutError int

// Error implements the Error interface.
func (t TimeoutError) Error() string {
	return fmt.Sprintf("expect: timer expired after %d seconds", time.Duration(t)/time.Second)
}

// Tag represents the state for a Caser.
type Tag int32

const (
	// OKTag marks the desired state was reached.
	OKTag = Tag(iota)
	// NoTag signals no tag was set for this case.
	NoTag
)

// OK returns the OK Tag.
func OK() func() (Tag, error) {
	return func() (Tag, error) {
		return OKTag, nil
	}
}

// Caser is an interface for ExpectSwitchCase.
type Caser interface {
	// RE returns a compiled regexp
	RE() (*regexp.Regexp, error)
	// String returns the send string
	String() string
	// Tag returns the Tag.
	Tag() (Tag, error)
}

// Case used by the ExpectSwitchCase to take different Cases.
// Implements the Caser interface.
type Case struct {
	// R is the compiled regexp to match.
	R *regexp.Regexp
	// S is the string to send if Regexp matches.
	S string
	// T is the Tag for this Case.
	T func() (Tag, error)
}

// Tag returns the tag for this case.
func (c *Case) Tag() (Tag, error) {
	if c.T == nil {
		return NoTag, nil
	}
	return c.T()
}

// RE returns the compiled regular expression.
func (c *Case) RE() (*regexp.Regexp, error) {
	return c.R, nil
}

// Send returns the string to send if regexp matches
func (c *Case) String() string {
	return c.S
}

// GExpect implements the spawning and interaction logic.
type GExpect struct {
	// pty holds the virtual terminal used to interact with the spawned commands.
	pty *term.PTY
	// cmd contains the cmd information for the spawned process.
	cmd *exec.Cmd
	// snd is the channel used by the Send command to send data into the spawned command.
	snd chan string
	// rcv is used to signal the Expect commands that new data arrived.
	rcv chan struct{}
	// chkMu lock protecting the check function.
	chkMu sync.RWMutex
	// chk contains the function to check if the spawned command is alive.
	chk func(*GExpect) bool
	// cls contains the function to close spawned command.
	cls func(*GExpect) error
	// timeout contains the default timeout for a spawned command.
	timeout time.Duration
	// sendTimeout contains the default timeout for a send command.
	sendTimeout time.Duration
	// chkDuration contains the duration between checks for new incoming data.
	chkDuration time.Duration
	// verbose enables verbose logging.
	verbose bool
	// verboseWriter if set specifies where to write verbose information.
	verboseWriter io.Writer
	// teeWriter receives a duplicate of the spawned process's output when set.
	teeWriter io.WriteCloser
	// bufferSize is the size of the io buffers in bytes.
	bufferSize int
	// bufferSizeIsSet tracks whether the bufferSize was set for a given GExpect instance.
	bufferSizeIsSet bool

	// mu protects the output buffer. It must be held for any operations on out.
	mu  sync.Mutex
	out bytes.Buffer
}

// String implements the stringer interface.
func (e *GExpect) String() string {
	res := fmt.Sprintf("%p: ", e)
	if e.pty != nil {
		_, name := e.pty.PTSName()
		res += fmt.Sprintf("pty: %s ", name)
	}
	if e.cmd != nil {
		res += fmt.Sprintf("cmd: %s(%d) ", e.cmd.Path, e.cmd.Process.Pid)
	}
	res += fmt.Sprintf("buf: %q", e.out.String())
	return res
}

func (e *GExpect) check() bool {
	e.chkMu.RLock()
	defer e.chkMu.RUnlock()
	return e.chk(e)
}

// Close closes the Spawned session.
func (e *GExpect) Close() error {
	return e.cls(e)
}

// Read implements the reader interface for the out buffer.
func (e *GExpect) Read(p []byte) (nr int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.out.Read(p)
}

// Send sends a string to spawned process.
func (e *GExpect) Send(in string) error {
	if !e.check() {
		return errors.New("expect: Process not running")
	}
	e.snd <- in
	return nil
}

// Spawn starts a new process and collects the output.
func Spawn(command string, timeout time.Duration, opts ...Option) (*GExpect, <-chan error, error) {
	return SpawnWithArgs(strings.Fields(command), timeout, opts...)
}

// SpawnWithArgs starts a new process and collects the output.
func SpawnWithArgs(command []string, timeout time.Duration, opts ...Option) (*GExpect, <-chan error, error) {
	pty, err := term.OpenPTY()
	if err != nil {
		return nil, nil, err
	}
	var t term.Termios
	t.Raw()
	t.Set(pty.Slave)

	if timeout < 1 {
		timeout = DefaultTimeout
	}
	// Get the command up and running
	cmd := exec.Command(command[0], command[1:]...)
	// This ties the commands Stdin,Stdout & Stderr to the virtual terminal we created
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pty.Slave, pty.Slave, pty.Slave
	// New process needs to be the process leader and control of a tty
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true}
	e := &GExpect{
		rcv:         make(chan struct{}),
		snd:         make(chan string),
		cmd:         cmd,
		timeout:     timeout,
		chkDuration: checkDuration,
		pty:         pty,
		cls: func(e *GExpect) error {
			if e.cmd != nil && e.cmd.Process != nil {
				return e.cmd.Process.Kill()
			}
			return nil
		},
		chk: func(e *GExpect) bool {
			if e.cmd.Process == nil {
				return false
			}
			return e.cmd.Process.Signal(syscall.Signal(0)) == nil
		},
	}
	for _, o := range opts {
		o(e)
	}

	// Set the buffer size to the default if expect.BufferSize(...) is not utilized.
	if !e.bufferSizeIsSet {
		e.bufferSize = defaultBufferSize
	}

	res := make(chan error, 1)
	go e.runcmd(res)
	// Wait until command started
	return e, res, <-res
}

// runcmd executes the command and Wait for the return value.
func (e *GExpect) runcmd(res chan error) {
	if err := e.cmd.Start(); err != nil {
		res <- err
		return
	}
	clean := make(chan struct{})
	chDone := e.goIO(clean)
	// Signal command started
	res <- nil
	cErr := e.cmd.Wait()
	close(chDone)
	e.pty.Slave.Close()
	// make sure the read/send routines are done before closing the pty.
	<-clean
	res <- cErr
}

// goIO starts the io handlers.
func (e *GExpect) goIO(clean chan struct{}) (done chan struct{}) {
	done = make(chan struct{})
	var ptySync sync.WaitGroup
	ptySync.Add(2)
	go e.read(done, &ptySync)
	go e.send(done, &ptySync)
	go func() {
		ptySync.Wait()
		e.pty.Master.Close()
		close(clean)
	}()
	return done
}

// read reads from the PTY master and forwards to active Expect function.
func (e *GExpect) read(done chan struct{}, ptySync *sync.WaitGroup) {
	defer ptySync.Done()
	buf := make([]byte, e.bufferSize)
	for {
		nr, err := e.pty.Master.Read(buf)
		if err != nil && !e.check() {
			if e.teeWriter != nil {
				e.teeWriter.Close()
			}
			return
		}
		// Tee output to writer
		if e.teeWriter != nil {
			e.teeWriter.Write(buf[:nr])
		}
		// Add to buffer
		e.mu.Lock()
		e.out.Write(buf[:nr])
		e.mu.Unlock()
		// Ping Expect function
		select {
		case e.rcv <- struct{}{}:
		default:
		}
	}
}

// send writes to the PTY master.
func (e *GExpect) send(done chan struct{}, ptySync *sync.WaitGroup) {
	defer ptySync.Done()
	for {
		select {
		case <-done:
			return
		case s := <-e.snd:
			for n, bytesWritten, err := 0, 0, error(nil); bytesWritten < len(s); bytesWritten += n {
				n, err = e.pty.Master.Write([]byte(s)[bytesWritten:])
				if err != nil {
					log.Printf("Write to PTY master failed: %v", err)
					break
				}
			}
		}
	}
}

// Expect reads spawned processes output looking for input regular expression.
func (e *GExpect) Expect(re *regexp.Regexp, timeout time.Duration) (string, []string, error) {
	out, match, _, err := e.ExpectSwitchCase([]Caser{&Case{re, "", nil}}, timeout)
	return out, match, err
}

// ExpectSwitchCase makes it possible to Expect with multiple regular expressions and actions.
func (e *GExpect) ExpectSwitchCase(cs []Caser, timeout time.Duration) (string, []string, int, error) {
	// Compile all regexps
	rs := make([]*regexp.Regexp, 0, len(cs))
	for _, c := range cs {
		re, err := c.RE()
		if err != nil {
			return "", []string{""}, -1, err
		}
		rs = append(rs, re)
	}
	if timeout < 0 {
		timeout = e.timeout
	}
	timer := time.NewTimer(timeout)
	check := e.chkDuration
	if timeout>>2 < check {
		check = timeout >> 2
		if check <= 0 {
			check = 1
		}
	}
	chTicker := time.NewTicker(check)
	defer chTicker.Stop()

	// Read in current data and start actively check for matches.
	var tbuf bytes.Buffer
	if _, err := io.Copy(&tbuf, e); err != nil {
		return tbuf.String(), nil, -1, fmt.Errorf("io.Copy failed: %v", err)
	}
	for {
		for i, c := range cs {
			if rs[i] == nil {
				continue
			}
			match := rs[i].FindStringSubmatch(tbuf.String())
			if match == nil {
				continue
			}

			_, err := c.Tag()
			if err != nil {
				return tbuf.String(), nil, -1, err
			}

			if e.verbose {
				if e.verboseWriter != nil {
					vStr := fmt.Sprintf("Match for RE: %q found: %q Buffer: %s\n", rs[i].String(), match, tbuf.String())
					e.verboseWriter.Write([]byte(vStr))
				} else {
					log.Printf("Match for RE: %q found: %q Buffer: %q", rs[i].String(), match, tbuf.String())
				}
			}

			tbufString := tbuf.String()
			o := tbufString
			tbuf.Reset()

			st := c.String()
			if len(match) > 1 && len(st) > 0 {
				for i := 1; i < len(match); i++ {
					si := strconv.Itoa(i)
					r := strings.NewReplacer(`\\`+si, `\`+si, `\`+si, `\\`+si)
					st = r.Replace(st)
					st = strings.Replace(st, `\\`+si, match[i], -1)
				}
			}
			if st != "" {
				if err := e.Send(st); err != nil {
					return o, match, i, fmt.Errorf("failed to send: %q err: %v", st, err)
				}
			}
			return o, match, i, nil
		}
		if !e.check() {
			nr, err := io.Copy(&tbuf, e)
			if err != nil {
				return tbuf.String(), nil, -1, fmt.Errorf("io.Copy failed: %v", err)
			}
			if nr == 0 {
				return tbuf.String(), nil, -1, errors.New("expect: Process not running")
			}
		}
		select {
		case <-timer.C:
			// Expect timeout.
			nr, err := io.Copy(&tbuf, e)
			if err != nil {
				return tbuf.String(), nil, -1, fmt.Errorf("io.Copy failed: %v", err)
			}
			if nr == 0 {
				return tbuf.String(), nil, -1, TimeoutError(timeout)
			}
			timer = time.NewTimer(timeout)
		case <-chTicker.C:
			// Periodical timer
			if _, err := io.Copy(&tbuf, e); err != nil {
				return tbuf.String(), nil, -1, fmt.Errorf("io.Copy failed: %v", err)
			}
		case <-e.rcv:
			// Data to fetch.
			nr, err := io.Copy(&tbuf, e)
			if err != nil {
				return tbuf.String(), nil, -1, fmt.Errorf("io.Copy failed: %v", err)
			}
			if nr == 0 {
				select {
				case <-time.After(10 * time.Millisecond):
				}
			}
		}
	}
}
