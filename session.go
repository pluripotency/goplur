package goplur

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/goexpect"
	"golang.org/x/term"
)

type ReactionType int

const (
	ReactionSuccess ReactionType = iota
	ReactionSend
	ReactionSendLine
	ReactionSendPass
	ReactionGetPass
	ReactionSendControl
	ReactionExit
	ReactionCapture
)

type ExpectRow struct {
	Pattern  string
	Reaction ReactionType
	Arg      interface{}
	Label    string
}

type nextAction int

const (
	nextContinue nextAction = iota
	nextBreak
)

type Session struct {
	child          *expect.GExpect
	nodes          []*Node
	timeout        time.Duration
	defaultTimeout time.Duration
	logger         *SessionLogger
}

func SelectLogParams(envStr string) LogParams {
	now := time.Now()
	ymd := now.Format("20060102")
	hmsF := now.Format("150405_000") // %H%M%S_%f approximation
	logDir := "/tmp/plur_log"

	switch envStr {
	case "only_stdout":
		return LogParams{EnableStdout: true}
	case "normal_on_tmp", "normal":
		return LogParams{
			LogDir:            logDir,
			EnableStdout:      true,
			OutputLogFilePath: filepath.Join(logDir, ymd, "output_"+hmsF+".log"),
			DebugColor:        true,
			DebugLogFilePath:  filepath.Join(logDir, ymd, "debug_"+hmsF+".log"),
			DeleteMtimeUnit:   "day",
			DeleteMtime:       10,
		}
	case "append_on_tmp", "append":
		lp := SelectLogParams("normal")
		lp.OutputLogAppendPath = filepath.Join(logDir, "output_append.log")
		lp.DebugLogAppendPath = filepath.Join(logDir, "debug_append.log")
		return lp
	case "debug_on_tmp", "debug":
		lp := SelectLogParams("append")
		lp.DontTruncate = true
		return lp
	case "silent":
		return LogParams{}
	default:
		// Default when env variable is not specified or fallback
		return LogParams{EnableStdout: true}
	}
}

func NewSession(node *Node, logParams *LogParams) *Session {
	var lp LogParams
	if logParams != nil {
		lp = *logParams
	} else {
		envVal := os.Getenv("LOG_PARAMS")
		if envVal == "" {
			envVal = "only_stdout"
		}
		lp = SelectLogParams(envVal)
	}

	s := &Session{
		timeout:        600 * time.Second,
		defaultTimeout: 600 * time.Second,
		logger:         NewSessionLogger(lp),
	}
	s.PushNode(node)
	return s
}

func (s *Session) PushNode(node *Node) {
	s.nodes = append(s.nodes, node)
}

func (s *Session) PopNode() {
	if len(s.nodes) > 1 {
		s.nodes = s.nodes[:len(s.nodes)-1]
	}
}

func (s *Session) CurrentNode() *Node {
	if len(s.nodes) == 0 {
		return nil
	}
	return s.nodes[len(s.nodes)-1]
}

func (s *Session) SetTimeout(t time.Duration) {
	s.timeout = t
}

func (s *Session) SetDefaultTimeout(t time.Duration) {
	s.defaultTimeout = t
}

func (s *Session) Close() {
	s.logger.debugLog.Message("INFO:Closing session.")
	s.logger.Close()
	if s.child != nil {
		s.child.Close()
	}
}

func (s *Session) actionHandler(action string) error {
	if s.child == nil {
		s.logger.debugLog.Message(fmt.Sprintf("Spawning by: %s", action))
		exp, _, err := expect.Spawn(action, s.timeout, expect.Verbose(true), expect.VerboseWriter(s.logger.outputWriter))
		if err != nil {
			return err
		}
		s.child = exp
	} else {
		s.logger.debugLog.Message(fmt.Sprintf("Sending action: %s", action))
		err := s.child.Send(action + "\n")
		if err != nil {
			return err
		}
	}
	s.logger.debugLog.onAction(s, action)
	return nil
}

func (s *Session) executeReaction(row ExpectRow, matches []string, out string) (interface{}, nextAction, error) {
	switch row.Reaction {
	case ReactionSuccess:
		s.logger.debugLog.atRowMethod(fmt.Sprintf("success returns: %v", row.Arg))
		return row.Arg, nextBreak, nil

	case ReactionCapture:
		s.logger.debugLog.atRowMethod("p_capture returns: child.before")
		pattern := row.Pattern
		if pattern == "" || pattern == "waitprompt" {
			pattern = s.CurrentNode().WaitPrompt
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return out, nextBreak, nil
		}
		loc := re.FindStringIndex(out)
		if loc != nil {
			before := out[:loc[0]]
			return before, nextBreak, nil
		}
		return out, nextBreak, nil

	case ReactionSend:
		str := row.Arg.(string)
		s.logger.debugLog.atRowMethod(fmt.Sprintf("send sent: %s", str))
		err := s.child.Send(str)
		return nil, nextContinue, err

	case ReactionSendLine:
		str := row.Arg.(string)
		s.logger.debugLog.atRowMethod(fmt.Sprintf("send_line sent: %s", str))
		err := s.child.Send(str + "\n")
		return nil, nextContinue, err

	case ReactionSendPass:
		s.logger.debugLog.atRowMethod("send_pass sent: (password omit)")
		s.logger.outputWriter.Mute()

		password := ""
		if str, ok := row.Arg.(string); ok && str != "" {
			password = str
		} else {
			password = s.CurrentNode().Password
		}

		err := s.child.Send(password + "\n")
		s.logger.outputWriter.Unmute()
		return nil, nextContinue, err

	case ReactionGetPass:
		s.logger.debugLog.atRowMethod("get_pass/prompting manual password: (password omit)")
		s.logger.outputWriter.Mute()

		fmt.Fprintf(os.Stderr, "Password: ")
		passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			s.logger.outputWriter.Unmute()
			return nil, nextBreak, err
		}

		err = s.child.Send(string(passBytes) + "\n")
		s.logger.outputWriter.Unmute()
		return nil, nextContinue, err

	case ReactionSendControl:
		char := row.Arg.(string)
		ctrlStr := getControlChar(char)
		s.logger.debugLog.atRowMethod(fmt.Sprintf("send_control sent: Ctrl-%s", char))
		err := s.child.Send(ctrlStr)
		return nil, nextContinue, err

	case ReactionExit:
		s.logger.debugLog.Message("exit reaction triggered")
		os.Exit(1)
		return nil, nextBreak, fmt.Errorf("exit reaction triggered")
	}

	return nil, nextBreak, fmt.Errorf("unknown reaction type")
}

func getControlChar(char string) string {
	if len(char) != 1 {
		return ""
	}
	c := char[0]
	if c >= 'a' && c <= 'z' {
		return string(c - 'a' + 1)
	}
	if c >= 'A' && c <= 'Z' {
		return string(c - 'A' + 1)
	}
	return ""
}

func (s *Session) Do(action string, rows []ExpectRow, timeout time.Duration) (interface{}, error) {
	err := s.actionHandler(action)
	if err != nil {
		return nil, err
	}

	if timeout == 0 {
		timeout = s.timeout
	}

	for {
		var cs []expect.Caser
		var normalizedPatterns []string

		for _, row := range rows {
			pattern := row.Pattern
			if pattern == "" || pattern == "waitprompt" {
				pattern = s.CurrentNode().WaitPrompt
			}
			normalizedPatterns = append(normalizedPatterns, pattern)

			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q: %v", pattern, err)
			}

			cs = append(cs, &expect.Case{
				R: re,
				T: expect.OK(),
			})
		}

		s.logger.debugLog.beforeSelect(s, rows, normalizedPatterns)

		out, submatches, idx, err := s.child.ExpectSwitchCase(cs, timeout)
		if err != nil {
			isTimeout := strings.Contains(err.Error(), "timer expired") || strings.Contains(err.Error(), "timeout")
			isEOF := err == io.EOF || strings.Contains(err.Error(), "EOF")

			if isTimeout {
				for idx, row := range rows {
					if row.Pattern == "timeout" {
						s.logger.debugLog.afterSelect(idx, "TIMEOUT")
						res, _, rErr := s.executeReaction(row, nil, out)
						return res, rErr
					}
				}
			}
			if isEOF {
				for idx, row := range rows {
					if row.Pattern == "EOF" {
						s.logger.debugLog.afterSelect(idx, "EOF")
						res, _, rErr := s.executeReaction(row, nil, out)
						return res, rErr
					}
				}
			}
			return nil, err
		}

		s.logger.debugLog.afterSelect(idx, out)

		matchedRow := rows[idx]
		res, next, err := s.executeReaction(matchedRow, submatches, out)
		if err != nil {
			return nil, err
		}

		if next == nextBreak {
			return res, nil
		}
	}
}

func (s *Session) Run(command string) (string, error) {
	res, err := s.Do(command, []ExpectRow{
		{Pattern: "", Reaction: ReactionCapture, Label: command},
	}, s.timeout)
	if err != nil {
		return "", err
	}
	if str, ok := res.(string); ok {
		return str, nil
	}
	return "", fmt.Errorf("unexpected return type from Run")
}

func (s *Session) langC() error {
	_, err := s.Run("stty -echo")
	if err != nil {
		return err
	}
	_, err = s.Run("export LANG=C PROMPT_COMMAND=\"\"")
	return err
}

func (s *Session) platformRun() error {
	node := s.CurrentNode()
	if node.Platform == "" {
		return nil
	}

	isRhelAlmaCentos := false
	for _, p := range []string{"almalinux8", "almalinux9", "centos8", "centos9"} {
		if node.Platform == p {
			isRhelAlmaCentos = true
			break
		}
	}

	if isRhelAlmaCentos {
		err := s.langC()
		if err != nil {
			return err
		}
		_, err = s.Run("bind 'set enable-bracketed-paste off'")
		return err
	}

	matched, _ := regexp.MatchString("centos|rhel|fedora|ubuntu", node.Platform)
	if matched {
		return s.langC()
	}
	return nil
}

func (s *Session) Bash() (*Session, error) {
	node := s.CurrentNode()
	node.ExitCommand = "exit"

	_, err := s.Do("bash", []ExpectRow{
		{Pattern: "", Reaction: ReactionCapture},
	}, s.timeout)
	if err != nil {
		return nil, err
	}

	err = s.platformRun()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) expectsOnLogin(password string) []ExpectRow {
	return []ExpectRow{
		{Pattern: `Are you sure you want to continue connecting \(yes/no.+\?`, Reaction: ReactionSendLine, Arg: "yes", Label: "SSH confirmation"},
		{Pattern: `[Pp]assword:`, Reaction: ReactionSendPass, Arg: password, Label: "Password prompt"},
		{Pattern: `Permission denied, please try again.+password:`, Reaction: ReactionGetPass, Label: "Password retry"},
		{Pattern: `Permission denied \(publickey,`, Reaction: ReactionExit, Label: "SSH public key denied"},
		{Pattern: `WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!`, Reaction: ReactionExit, Label: "SSH host key changed"},
		{Pattern: `ssh: Could not resolve hostname`, Reaction: ReactionExit, Label: "SSH hostname error"},
		{Pattern: `ssh: connect to host `, Reaction: ReactionExit, Label: "SSH connection error"},
		{Pattern: "", Reaction: ReactionCapture, Label: "Prompt capture"},
	}
}

func (s *Session) Ssh() (*Session, error) {
	node := s.CurrentNode()
	node.ExitCommand = "exit"

	accessTarget := node.AccessIP
	if accessTarget == "" {
		accessTarget = node.Hostname
	}
	if accessTarget == "" {
		return nil, fmt.Errorf("ssh: access ip or hostname is required")
	}

	if node.Username == "" {
		return nil, fmt.Errorf("ssh: username is required")
	}

	action := fmt.Sprintf("ssh %s@%s", node.Username, accessTarget)
	if node.SSHPort != 0 && node.SSHPort != 22 {
		action += fmt.Sprintf(" -p %d", node.SSHPort)
	}
	if node.SSHOptions != "" {
		action += " " + node.SSHOptions
	}

	_, err := s.Do(action, s.expectsOnLogin(node.Password), s.timeout)
	if err != nil {
		return nil, err
	}

	err = s.platformRun()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) Telnet() (*Session, error) {
	node := s.CurrentNode()
	node.ExitCommand = "exit"

	accessTarget := node.AccessIP
	if accessTarget == "" {
		accessTarget = node.Hostname
	}
	if accessTarget == "" {
		return nil, fmt.Errorf("telnet: access ip or hostname is required")
	}

	action := fmt.Sprintf("telnet %s", accessTarget)

	rows := s.expectsOnLogin(node.Password)
	loginRow := ExpectRow{Pattern: `[Ll]ogin:`, Reaction: ReactionSendLine, Arg: node.Username, Label: "Telnet login username"}
	rows = append([]ExpectRow{loginRow}, rows...)

	_, err := s.Do(action, rows, s.timeout)
	if err != nil {
		return nil, err
	}

	err = s.platformRun()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) findRootPassword() string {
	for i := len(s.nodes) - 1; i >= 0; i-- {
		node := s.nodes[i]
		if node.RootPassword != "" {
			return node.RootPassword
		}
		if node.Username == "root" && node.Password != "" {
			return node.Password
		}
	}
	return ""
}

func (s *Session) Su(username string) (*Session, error) {
	if username == "" {
		username = "root"
	}
	currentNode := s.CurrentNode()
	if currentNode.Username == username {
		return s, nil
	}

	suNode := &Node{
		Hostname:     currentNode.Hostname,
		Username:     username,
		Platform:     currentNode.Platform,
		RootPassword: currentNode.RootPassword,
		Password:     currentNode.Password,
		WaitPrompt:   GetLinuxWaitprompt(currentNode.Platform, currentNode.Hostname, username),
	}

	action := "su - " + username

	rows := []ExpectRow{
		{Pattern: suNode.WaitPrompt, Reaction: ReactionSuccess, Arg: true, Label: "SU target prompt"},
		{Pattern: currentNode.WaitPrompt, Reaction: ReactionSuccess, Arg: false, Label: "SU current prompt (failure)"},
	}

	if currentNode.Username == "root" {
		res, err := s.Do(action, rows, s.timeout)
		if err != nil {
			return nil, err
		}
		if res == true {
			s.PushNode(suNode)
			err = s.platformRun()
			if err != nil {
				return nil, err
			}
			return s, nil
		}
		return nil, fmt.Errorf("su failed to switch to %s", username)
	}

	foundPassword := s.findRootPassword()
	if foundPassword != "" {
		rows = append(rows, ExpectRow{Pattern: `[Pp]assword:`, Reaction: ReactionSendPass, Arg: foundPassword, Label: "SU password prompt"})
	} else {
		rows = append(rows, ExpectRow{Pattern: `[Pp]assword:`, Reaction: ReactionGetPass, Label: "SU password manual input"})
	}

	res, err := s.Do(action, rows, s.timeout)
	if err != nil {
		return nil, err
	}
	if res == true {
		s.PushNode(suNode)
		err = s.platformRun()
		if err != nil {
			return nil, err
		}
		return s, nil
	}
	return nil, fmt.Errorf("su failed to switch to %s", username)
}

func (s *Session) addSudoer(username string) error {
	currentNode := s.CurrentNode()
	if currentNode.Username == "root" {
		_, err := s.Run(fmt.Sprintf(`echo "%s ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/%s`, username, username))
		return err
	}
	_, err := s.Su("root")
	if err != nil {
		return err
	}
	err = s.addSudoer(username)
	if err != nil {
		return err
	}
	_, err = s.SuExit()
	return err
}

func (s *Session) SudoI() (*Session, error) {
	currentNode := s.CurrentNode()
	suNode := &Node{
		Hostname:     currentNode.Hostname,
		Username:     "root",
		Platform:     currentNode.Platform,
		RootPassword: currentNode.RootPassword,
		Password:     currentNode.Password,
		WaitPrompt:   GetLinuxWaitprompt(currentNode.Platform, currentNode.Hostname, "root"),
	}

	if strings.HasPrefix(currentNode.Platform, "ubuntu") {
		rows := []ExpectRow{
			{Pattern: suNode.WaitPrompt, Reaction: ReactionSuccess, Arg: true, Label: "Sudo elevation success"},
			{Pattern: currentNode.WaitPrompt, Reaction: ReactionSuccess, Arg: false, Label: "Sudo elevation failure"},
			{Pattern: `\[sudo\] password for`, Reaction: ReactionSendPass, Arg: currentNode.Password, Label: "Sudo password prompt"},
			{Pattern: `Sorry, try again.+password for`, Reaction: ReactionGetPass, Label: "Sudo password manual retry"},
		}
		res, err := s.Do("sudo -i", rows, s.timeout)
		if err != nil {
			return nil, err
		}
		if res == true {
			s.PushNode(suNode)
			err = s.platformRun()
			if err != nil {
				return nil, err
			}
			return s, nil
		}
		return nil, fmt.Errorf("sudo_i failed")
	}

	err := s.actionHandler("sudo -i")
	if err != nil {
		return nil, err
	}

	cs := []expect.Caser{
		&expect.Case{R: regexp.MustCompile(suNode.WaitPrompt), T: expect.OK()},
		&expect.Case{R: regexp.MustCompile(currentNode.WaitPrompt), T: expect.OK()},
		&expect.Case{R: regexp.MustCompile(`\[sudo\] password for`), T: expect.OK()},
	}

	out, _, idx, err := s.child.ExpectSwitchCase(cs, s.timeout)
	if err != nil {
		return nil, err
	}

	s.logger.debugLog.afterSelect(idx, out)

	if idx == 0 {
		s.PushNode(suNode)
		err = s.platformRun()
		if err != nil {
			return nil, err
		}
		return s, nil
	}

	if idx == 2 {
		err = s.child.Send(getControlChar("c"))
		if err != nil {
			return nil, err
		}
		_, _, err = s.child.Expect(regexp.MustCompile(currentNode.WaitPrompt), s.timeout)
		if err != nil {
			return nil, err
		}

		err = s.addSudoer(currentNode.Username)
		if err != nil {
			return nil, err
		}

		return s.SudoI()
	}

	return nil, fmt.Errorf("sudo_i failed")
}

func (s *Session) SuExit() (*Session, error) {
	if len(s.nodes) <= 1 {
		return s, nil
	}
	s.PopNode()
	_, err := s.Run("exit")
	return s, err
}
