package node

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
)

type Node interface {
	GetHostname() string
	GetUsername() string
	GetPassword() string
	GetPlatform() string
	GetWaitPrompt() string
	GetAccessIP() string
	GetExitCommand() string
	GetRootPassword() string
}

type BaseNode struct {
	Hostname     string `json:"hostname"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Platform     string `json:"platform"`
	WaitPrompt   string `json:"waitprompt"`
	AccessIP     string `json:"access_ip"`
	ExitCommand  string `json:"exit_command"`
	RootPassword string `json:"root_password"`
}

func (n *BaseNode) GetHostname() string     { return n.Hostname }
func (n *BaseNode) GetUsername() string     { return n.Username }
func (n *BaseNode) GetPassword() string     { return n.Password }
func (n *BaseNode) GetPlatform() string     { return n.Platform }
func (n *BaseNode) GetWaitPrompt() string   { return n.WaitPrompt }
func (n *BaseNode) GetAccessIP() string     { return n.AccessIP }
func (n *BaseNode) GetRootPassword() string { return n.RootPassword }

func (n *BaseNode) GetExitCommand() string {
	if n.ExitCommand == "" {
		return "exit"
	}
	return n.ExitCommand
}

type BashNode struct {
	Hostname    string `json:"hostname"`
	Username    string `json:"username"`
	Platform    string `json:"platform"`
	WaitPrompt  string `json:"waitprompt"`
	ExitCommand string `json:"exit_command"`
}

func (n *BashNode) GetHostname() string     { return n.Hostname }
func (n *BashNode) GetUsername() string     { return n.Username }
func (n *BashNode) GetPassword() string     { return "" }
func (n *BashNode) GetPlatform() string     { return n.Platform }
func (n *BashNode) GetWaitPrompt() string   { return n.WaitPrompt }
func (n *BashNode) GetAccessIP() string     { return "" }
func (n *BashNode) GetRootPassword() string { return "" }

func (n *BashNode) GetExitCommand() string {
	if n.ExitCommand == "" {
		return "exit"
	}
	return n.ExitCommand
}

type TelnetNode struct {
	BaseNode
	TelnetPort int `json:"telnet_port"`
}

func (n *TelnetNode) GetTelnetPort() int { return n.TelnetPort }

type SshNode struct {
	BaseNode
	SSHPort    int    `json:"ssh_port"`
	SSHOptions string `json:"ssh_options"`
}

func (n *SshNode) GetSSHPort() int       { return n.SSHPort }
func (n *SshNode) GetSSHOptions() string { return n.SSHOptions }

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
