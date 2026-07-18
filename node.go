package goplur

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
)

type Node struct {
	Hostname     string `json:"hostname"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Platform     string `json:"platform"`
	WaitPrompt   string `json:"waitprompt"`
	AccessIP     string `json:"access_ip"`
	SSHPort      int    `json:"ssh_port"`
	SSHOptions   string `json:"ssh_options"`
	ExitCommand  string `json:"exit_command"`
	RootPassword string `json:"root_password"`
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

func NewMeNode() *Node {
	hostname, _ := os.Hostname()
	if idx := strings.Index(hostname, "."); idx != -1 {
		hostname = hostname[:idx]
	}
	u, _ := user.Current()
	username := u.Username
	platform := DetectPlatform()
	return &Node{
		Hostname:   hostname,
		Username:   username,
		Platform:   platform,
		WaitPrompt: GetLinuxWaitprompt(platform, hostname, username),
	}
}

func NewSshNode(hostname, accessIP, username, password, platform string) *Node {
	return &Node{
		Hostname:   hostname,
		AccessIP:   accessIP,
		Username:   username,
		Password:   password,
		Platform:   platform,
		WaitPrompt: GetLinuxWaitprompt(platform, hostname, username),
		SSHPort:    22,
	}
}
