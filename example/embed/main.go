package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"

	"goplur"
)

//go:embed hosts.json
var embeddedHostsData []byte

//go:embed keys/*
var embeddedKeysFS embed.FS

type HostConfig struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"` // "ssh" or "telnet"
	AccessIP       string   `json:"access_ip"`
	Port           int      `json:"port"`
	Username       string   `json:"username"`
	AuthType       string   `json:"auth_type"` // "key" or "password"
	KeyFile        string   `json:"key_file"`  // embeddedKeysFS 内のパス
	Password       string   `json:"password"`
	EnablePassword string   `json:"enable_password"` // Cisco 特権モード用
	UseLoginFlag   bool     `json:"use_login_flag"`
	EscapeExit     bool     `json:"escape_exit"` // Telnet Ctrl+] 切断
	Platform       string   `json:"platform"`
	Tags           []string `json:"tags"`
}

func main() {
	var hosts []HostConfig
	if err := json.Unmarshal(embeddedHostsData, &hosts); err != nil {
		log.Fatalf("Failed to parse embedded hosts.json: %v", err)
	}

	if len(hosts) == 0 {
		log.Fatal("No hosts configured in embedded hosts.json")
	}

	selectedHost, err := selectHostInteractive(hosts)
	if err != nil {
		fmt.Printf("\nSearch cancelled: %v\n", err)
		return
	}

	fmt.Printf("\n[+] Target Selected: %s (%s)\n", selectedHost.Name, selectedHost.AccessIP)
	fmt.Printf("[+] Protocol: %s, User: %s, Port: %d\n", selectedHost.Type, selectedHost.Username, resolvePort(selectedHost))
	fmt.Println("[+] Connecting automatically without asking for credentials...")

	switch selectedHost.Type {
	case "ssh":
		err = connectSSH(selectedHost)
	case "telnet":
		err = connectTelnet(selectedHost)
	default:
		log.Fatalf("Unsupported host type: %s", selectedHost.Type)
	}

	if err != nil {
		log.Fatalf("\nConnection failed: %v", err)
	}
	fmt.Println("\n[+] Session disconnected successfully.")
}

func resolvePort(h HostConfig) int {
	if h.Port != 0 {
		return h.Port
	}
	if h.Type == "telnet" {
		return 23
	}
	return 22
}

func connectSSH(host HostConfig) error {
	node := goplur.NewSshNode(host.ID, host.AccessIP, host.Username, host.Password, host.Platform)
	if host.Port != 0 {
		node.SSHPort = host.Port
	}
	if host.UseLoginFlag {
		node.WithLoginFlag(true)
	}

	if host.AuthType == "key" && host.KeyFile != "" {
		keyBytes, err := embeddedKeysFS.ReadFile(host.KeyFile)
		if err != nil {
			return fmt.Errorf("embedded key %q not found: %w", host.KeyFile, err)
		}

		tmpFile, err := os.CreateTemp("", "goplur-key-*")
		if err != nil {
			return fmt.Errorf("failed to create temp key file: %w", err)
		}
		tmpPath := tmpFile.Name()

		defer os.Remove(tmpPath)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			_ = os.Remove(tmpPath)
			os.Exit(1)
		}()

		if err := os.Chmod(tmpPath, 0600); err != nil {
			return err
		}
		if _, err := tmpFile.Write(keyBytes); err != nil {
			return err
		}
		_ = tmpFile.Close()

		node.WithKey(tmpPath)
	}

	logParams := goplur.DefaultLogParams()

	return goplur.RunSsh(node, &logParams, func(s *goplur.Session) error {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf(" Connected to %s via SSH. Interactive shell ready.\n", host.Name)
		fmt.Println(" Type 'exit' or press Ctrl+D to disconnect.")
		fmt.Println("--------------------------------------------------------------------------------")
		return s.Interact()
	})
}

func connectTelnet(host HostConfig) error {
	node := goplur.NewTelnetNode(host.ID, host.AccessIP, host.Username, host.Password, host.Platform)
	if host.Port != 0 {
		node.TelnetPort = host.Port
	}

	if host.EscapeExit {
		node.WithEscapeExit()
	}

	if host.EnablePassword != "" {
		node.WaitPrompt = fmt.Sprintf(`%s#`, host.ID)
		node.ConnectHandler = func(s goplur.SessionExecutor, n goplur.Node) error {
			sess := s.(*goplur.Session)
			if err := sess.DefaultTelnetLogin(n); err != nil {
				return err
			}
			rows := []goplur.ExpectRow{
				{Pattern: `[Pp]assword:`, Reaction: goplur.ReactionSendPass, Arg: host.EnablePassword, Label: "enable pass"},
				{Pattern: node.GetWaitPrompt(), Reaction: goplur.ReactionSuccess, Arg: true, Label: "privileged prompt"},
			}
			_, err := sess.Do("enable", rows, 5*goplur.DefaultTimeout)
			return err
		}
		node.ExitHandler = func(s goplur.SessionExecutor, _ goplur.Node) error {
			sess := s.(*goplur.Session)
			_, _ = sess.Run("disable")
			return sess.SendLine("exit")
		}
	}

	logParams := goplur.DefaultLogParams()

	return goplur.RunTelnet(node, &logParams, func(s *goplur.Session) error {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf(" Connected to %s via Telnet. Interactive terminal ready.\n", host.Name)
		if host.EscapeExit {
			fmt.Println(" Disconnect via exit or Telnet escape (Ctrl+], then quit).")
		}
		fmt.Println("--------------------------------------------------------------------------------")
		return s.Interact()
	})
}

func selectHostInteractive(allHosts []HostConfig) (HostConfig, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fallbackSelect(allHosts)
	}
	defer term.Restore(fd, oldState)

	var query strings.Builder
	cursorIdx := 0

	render := func() []HostConfig {
		fmt.Print("\r\x1b[2K\x1b[H")
		fmt.Print("\x1b[J")

		fmt.Print("=== QuickConnect Host Search (Type to filter, UP/DOWN to navigate, ENTER to select, ESC/Ctrl+C to quit) ===\r\n")
		fmt.Printf("Query> %s_\r\n\r\n", query.String())

		filtered := filterHosts(allHosts, query.String())
		if len(filtered) == 0 {
			fmt.Print("  [No matching hosts found]\r\n")
			return nil
		}

		if cursorIdx >= len(filtered) {
			cursorIdx = len(filtered) - 1
		}
		if cursorIdx < 0 {
			cursorIdx = 0
		}

		maxDisplay := 10
		start := 0
		if cursorIdx >= maxDisplay {
			start = cursorIdx - maxDisplay + 1
		}
		end := start + maxDisplay
		if end > len(filtered) {
			end = len(filtered)
		}

		for i := start; i < end; i++ {
			h := filtered[i]
			tagStr := strings.Join(h.Tags, ",")
			prefix := "  "
			if i == cursorIdx {
				prefix = "\x1b[7m> "
			}

			line := fmt.Sprintf("%-16s | %-6s | %-15s | %-10s | %-25s | [%s]",
				h.ID, strings.ToUpper(h.Type), h.AccessIP, h.Username, h.Name, tagStr)

			if i == cursorIdx {
				fmt.Printf("%s%s\x1b[0m\r\n", prefix, line)
			} else {
				fmt.Printf("%s%s\r\n", prefix, line)
			}
		}

		fmt.Printf("\r\nShowing %d/%d hosts (Index: %d)\r\n", len(filtered), len(allHosts), cursorIdx+1)
		return filtered
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		filtered := render()

		b, err := reader.ReadByte()
		if err != nil {
			return HostConfig{}, err
		}

		switch b {
		case 3:
			return HostConfig{}, fmt.Errorf("interrupted")
		case 13:
			if len(filtered) > 0 && cursorIdx < len(filtered) {
				term.Restore(fd, oldState)
				return filtered[cursorIdx], nil
			}
		case 127, 8:
			str := query.String()
			if len(str) > 0 {
				query.Reset()
				query.WriteString(str[:len(str)-1])
			}
		case 27:
			if reader.Buffered() >= 2 {
				b1, _ := reader.ReadByte()
				b2, _ := reader.ReadByte()
				if b1 == '[' {
					switch b2 {
					case 'A':
						cursorIdx--
						if cursorIdx < 0 {
							cursorIdx = 0
						}
					case 'B':
						cursorIdx++
						if len(filtered) > 0 && cursorIdx >= len(filtered) {
							cursorIdx = len(filtered) - 1
						}
					}
				}
			} else {
				return HostConfig{}, fmt.Errorf("escape pressed")
			}
		case 16:
			cursorIdx--
			if cursorIdx < 0 {
				cursorIdx = 0
			}
		case 14:
			cursorIdx++
			if len(filtered) > 0 && cursorIdx >= len(filtered) {
				cursorIdx = len(filtered) - 1
			}
		default:
			if b >= 32 && b <= 126 {
				query.WriteByte(b)
				cursorIdx = 0
			}
		}
	}
}

func filterHosts(hosts []HostConfig, queryString string) []HostConfig {
	trimmed := strings.TrimSpace(queryString)
	if trimmed == "" {
		return hosts
	}

	tokens := strings.Fields(strings.ToLower(trimmed))
	var result []HostConfig

	for _, h := range hosts {
		targetText := strings.ToLower(fmt.Sprintf("%s %s %s %s %s %s",
			h.ID, h.Name, h.AccessIP, h.Username, h.Type, strings.Join(h.Tags, " ")))

		matchAll := true
		for _, token := range tokens {
			if !strings.Contains(targetText, token) {
				matchAll = false
				break
			}
		}
		if matchAll {
			result = append(result, h)
		}
	}

	return result
}

func fallbackSelect(hosts []HostConfig) (HostConfig, error) {
	fmt.Println("Select a host by number:")
	for i, h := range hosts {
		fmt.Printf("[%d] %s (%s) [%s]\n", i+1, h.Name, h.AccessIP, h.Type)
	}
	fmt.Print("Enter number: ")
	var idx int
	if _, err := fmt.Scanln(&idx); err != nil || idx < 1 || idx > len(hosts) {
		return HostConfig{}, fmt.Errorf("invalid selection")
	}
	return hosts[idx-1], nil
}
