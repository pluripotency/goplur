package node

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
)

type SessionExecutor interface {
	Run(command string) (string, error)
	Send(str string) error
	SendLine(str string) error
	SendControl(char string) error
}

type ConnectHandlerFunc func(s SessionExecutor, n Node) error
type ExitHandlerFunc func(s SessionExecutor, n Node) error

type Node interface {
	GetHostname() string
	GetUsername() string
	GetPassword() string
	GetPlatform() string
	GetWaitPrompt() string
	GetAccessIP() string
	GetExitCommand() string
	GetRootPassword() string
	GetInteractPreCommand() string
	GetInteractPostCommand() string
	GetConnectHandler() ConnectHandlerFunc
	GetExitHandler() ExitHandlerFunc
}

type BaseNode struct {
	Hostname            string             `json:"hostname"`
	Username            string             `json:"username"`
	Password            string             `json:"password"`
	Platform            string             `json:"platform"`
	WaitPrompt          string             `json:"waitprompt"`
	AccessIP            string             `json:"access_ip"`
	ExitCommand         string             `json:"exit_command"`
	RootPassword        string             `json:"root_password"`
	InteractPreCommand  string             `json:"interact_pre_command"`
	InteractPostCommand string             `json:"interact_post_command"`
	ConnectHandler      ConnectHandlerFunc `json:"-"`
	ExitHandler         ExitHandlerFunc    `json:"-"`
}

func (n *BaseNode) GetHostname() string                    { return n.Hostname }
func (n *BaseNode) GetUsername() string                    { return n.Username }
func (n *BaseNode) GetPassword() string                    { return n.Password }
func (n *BaseNode) GetPlatform() string                    { return n.Platform }
func (n *BaseNode) GetWaitPrompt() string                  { return n.WaitPrompt }
func (n *BaseNode) GetAccessIP() string                    { return n.AccessIP }
func (n *BaseNode) GetRootPassword() string                { return n.RootPassword }
func (n *BaseNode) GetInteractPreCommand() string          { return n.InteractPreCommand }
func (n *BaseNode) GetInteractPostCommand() string         { return n.InteractPostCommand }
func (n *BaseNode) GetConnectHandler() ConnectHandlerFunc  { return n.ConnectHandler }
func (n *BaseNode) GetExitHandler() ExitHandlerFunc        { return n.ExitHandler }

func (n *BaseNode) SetConnectHandler(fn ConnectHandlerFunc) *BaseNode {
	n.ConnectHandler = fn
	return n
}

func (n *BaseNode) SetExitHandler(fn ExitHandlerFunc) *BaseNode {
	n.ExitHandler = fn
	return n
}

func (n *BaseNode) GetExitCommand() string {
	if n.ExitCommand == "" {
		return "exit"
	}
	return n.ExitCommand
}

type BashNode struct {
	Hostname            string             `json:"hostname"`
	Username            string             `json:"username"`
	Platform            string             `json:"platform"`
	WaitPrompt          string             `json:"waitprompt"`
	ExitCommand         string             `json:"exit_command"`
	InteractPreCommand  string             `json:"interact_pre_command"`
	InteractPostCommand string             `json:"interact_post_command"`
	ConnectHandler      ConnectHandlerFunc `json:"-"`
	ExitHandler         ExitHandlerFunc    `json:"-"`
}

func (n *BashNode) GetHostname() string                    { return n.Hostname }
func (n *BashNode) GetUsername() string                    { return n.Username }
func (n *BashNode) GetPassword() string                    { return "" }
func (n *BashNode) GetPlatform() string                    { return n.Platform }
func (n *BashNode) GetWaitPrompt() string                  { return n.WaitPrompt }
func (n *BashNode) GetAccessIP() string                    { return "" }
func (n *BashNode) GetRootPassword() string                { return "" }
func (n *BashNode) GetConnectHandler() ConnectHandlerFunc  { return n.ConnectHandler }
func (n *BashNode) GetExitHandler() ExitHandlerFunc        { return n.ExitHandler }

func (n *BashNode) GetExitCommand() string {
	if n.ExitCommand == "" {
		return "exit"
	}
	return n.ExitCommand
}

func (n *BashNode) GetInteractPreCommand() string {
	if n.InteractPreCommand != "" {
		return n.InteractPreCommand
	}
	return "stty echo"
}

func (n *BashNode) GetInteractPostCommand() string {
	if n.InteractPostCommand != "" {
		return n.InteractPostCommand
	}
	return "stty -echo"
}

type TelnetCommandProvider interface {
	GetTelnetCommand() string
}

type TelnetNode struct {
	BaseNode
	TelnetPort  int                        `json:"telnet_port"`
	CommandFunc func(n *TelnetNode) string `json:"-"`
}

func (n *TelnetNode) GetTelnetPort() int { return n.TelnetPort }

func (n *TelnetNode) GetTelnetCommand() string {
	if n.CommandFunc != nil {
		return n.CommandFunc(n)
	}

	target := n.GetAccessIP()
	if target == "" {
		target = n.GetHostname()
	}

	port := 23
	if n.TelnetPort != 0 {
		port = n.TelnetPort
	}

	cmd := fmt.Sprintf("telnet %s", target)
	if port != 23 {
		cmd += fmt.Sprintf(" %d", port)
	}
	return cmd
}

func (n *TelnetNode) WithCommandFunc(fn func(n *TelnetNode) string) *TelnetNode {
	n.CommandFunc = fn
	return n
}

func (n *TelnetNode) WithConnectHandler(fn ConnectHandlerFunc) *TelnetNode {
	n.ConnectHandler = fn
	return n
}

func (n *TelnetNode) WithExitHandler(fn ExitHandlerFunc) *TelnetNode {
	n.ExitHandler = fn
	return n
}

// TelnetEscapeExitHandler は Ctrl-] 送信後に quit を送信して切断するハンドラです。
func TelnetEscapeExitHandler(s SessionExecutor, _ Node) error {
	if err := s.SendControl("]"); err != nil {
		return err
	}
	return s.SendLine("quit")
}

// WithEscapeExit は TelnetEscapeExitHandler を ExitHandler として設定します。
func (n *TelnetNode) WithEscapeExit() *TelnetNode {
	n.ExitHandler = TelnetEscapeExitHandler
	return n
}

type SSHCommandProvider interface {
	GetSSHCommand() string
}

type SshNode struct {
	BaseNode
	SSHPort      int                     `json:"ssh_port"`
	SSHOptions   string                  `json:"ssh_options"`
	KeyPath      string                  `json:"key_path"`
	UseLoginFlag bool                    `json:"use_login_flag"`
	CommandFunc  func(n *SshNode) string `json:"-"`
}

func (n *SshNode) GetSSHPort() int       { return n.SSHPort }
func (n *SshNode) GetSSHOptions() string { return n.SSHOptions }
func (n *SshNode) GetKeyPath() string    { return n.KeyPath }
func (n *SshNode) GetUseLoginFlag() bool { return n.UseLoginFlag }

func (n *SshNode) GetSSHCommand() string {
	if n.CommandFunc != nil {
		return n.CommandFunc(n)
	}

	target := n.GetAccessIP()
	if target == "" {
		target = n.GetHostname()
	}

	var parts []string
	parts = append(parts, "ssh")

	if n.KeyPath != "" {
		parts = append(parts, "-i", n.KeyPath)
	}

	if n.SSHPort != 0 && n.SSHPort != 22 {
		parts = append(parts, "-p", fmt.Sprintf("%d", n.SSHPort))
	}

	if n.UseLoginFlag {
		if target != "" {
			parts = append(parts, target)
		}
		if n.GetUsername() != "" {
			parts = append(parts, "-l", n.GetUsername())
		}
	} else {
		if n.GetUsername() != "" && target != "" {
			parts = append(parts, fmt.Sprintf("%s@%s", n.GetUsername(), target))
		} else if target != "" {
			parts = append(parts, target)
		}
	}

	if n.SSHOptions != "" {
		parts = append(parts, n.SSHOptions)
	}

	return strings.Join(parts, " ")
}

func (n *SshNode) WithKey(keyPath string) *SshNode {
	n.KeyPath = keyPath
	return n
}

func (n *SshNode) WithLoginFlag(useLoginFlag bool) *SshNode {
	n.UseLoginFlag = useLoginFlag
	return n
}

func (n *SshNode) WithCommandFunc(fn func(n *SshNode) string) *SshNode {
	n.CommandFunc = fn
	return n
}

func (n *SshNode) WithConnectHandler(fn ConnectHandlerFunc) *SshNode {
	n.ConnectHandler = fn
	return n
}

func (n *SshNode) WithExitHandler(fn ExitHandlerFunc) *SshNode {
	n.ExitHandler = fn
	return n
}

func (n *SshNode) GetInteractPreCommand() string {
	if n.InteractPreCommand != "" {
		return n.InteractPreCommand
	}
	return "stty echo; stty sane"
}

func (n *SshNode) GetInteractPostCommand() string {
	if n.InteractPostCommand != "" {
		return n.InteractPostCommand
	}
	return "stty -echo"
}

func IsPlatformRHEL(platform string) bool {
	matched, _ := regexp.MatchString("centos|fedora|rhel|alma|rocky", platform)
	return matched
}

func IsPlatformSystemd(platform string) bool {
	matched, _ := regexp.MatchString("centos6", platform)
	return !matched
}

func getUserLinuxWaitprompt(platform, hostname, username string) string {
	if IsPlatformRHEL(platform) {
		return fmt.Sprintf(`\[?%s@%s .+\]\$ `, username, hostname)
	}
	return fmt.Sprintf(`%s@%s..+\$ `, username, hostname)
}

func getRootLinuxWaitprompt(platform, hostname string) string {
	if IsPlatformRHEL(platform) {
		return fmt.Sprintf(`\[?root@%s .+\]# `, hostname)
	}
	return fmt.Sprintf(`root@%s..+# `, hostname)
}

func GetLinuxWaitprompt(platform, hostname, username string) string {
	if username == "root" {
		return getRootLinuxWaitprompt(platform, hostname)
	}
	return getUserLinuxWaitprompt(platform, hostname, username)
}

func DetectPlatform() string {
	redhatReleasePath := "/etc/redhat-release"
	etcIssuePath := "/etc/issue"
	platform := "almalinux9"

	if data, err := os.ReadFile(redhatReleasePath); err == nil {
		content := string(data)
		if strings.Contains(content, "AlmaLinux release 10") {
			platform = "almalinux10"
		} else if strings.Contains(content, "AlmaLinux release 9") {
			platform = "almalinux9"
		} else if strings.Contains(content, "AlmaLinux release 8") {
			platform = "almalinux8"
		} else if strings.Contains(content, "CentOS Linux release 7") {
			platform = "centos7"
		} else if strings.Contains(content, "CentOS Linux release 6") {
			platform = "centos6"
		} else {
			platform = "almalinux9"
		}
	} else if data, err := os.ReadFile(etcIssuePath); err == nil {
		content := string(data)
		if strings.Contains(content, "Ubuntu 24.04") {
			platform = "ubuntu noble"
		} else if strings.Contains(content, "Ubuntu 26.04") {
			platform = "ubuntu resolute"
		} else if strings.Contains(content, "Ubuntu 22.04") {
			platform = "ubuntu jammy"
		} else if strings.Contains(content, "Ubuntu") {
			platform = "ubuntu"
		} else if strings.Contains(content, "Arch Linux") {
			platform = "arch"
		}
	}
	return platform
}

func NewMeNode() *BashNode {
	hostname, _ := os.Hostname()
	if idx := strings.Index(hostname, "."); idx != -1 {
		hostname = hostname[:idx]
	}
	u, _ := user.Current()
	username := u.Username
	platform := DetectPlatform()
	return &BashNode{
		Hostname:    hostname,
		Username:    username,
		Platform:    platform,
		WaitPrompt:  GetLinuxWaitprompt(platform, hostname, username),
		ExitCommand: "exit",
	}
}

func NewSshNode(hostname, accessIP, username, password, platform string) *SshNode {
	return &SshNode{
		BaseNode: BaseNode{
			Hostname:    hostname,
			AccessIP:    accessIP,
			Username:    username,
			Password:    password,
			Platform:    platform,
			WaitPrompt:  GetLinuxWaitprompt(platform, hostname, username),
			ExitCommand: "exit",
		},
		SSHPort: 22,
	}
}

func NewTelnetNode(hostname, accessIP, username, password, platform string) *TelnetNode {
	return &TelnetNode{
		BaseNode: BaseNode{
			Hostname:    hostname,
			AccessIP:    accessIP,
			Username:    username,
			Password:    password,
			Platform:    platform,
			WaitPrompt:  GetLinuxWaitprompt(platform, hostname, username),
			ExitCommand: "exit",
		},
		TelnetPort: 23,
	}
}
