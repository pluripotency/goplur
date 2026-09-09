package node

import (
	"fmt"
	"testing"
)

type mockSessionExecutor struct {
	runs     []string
	sends    []string
	sendLine []string
	controls []string
}

func (m *mockSessionExecutor) Run(cmd string) (string, error) {
	m.runs = append(m.runs, cmd)
	return "", nil
}

func (m *mockSessionExecutor) Send(str string) error {
	m.sends = append(m.sends, str)
	return nil
}

func (m *mockSessionExecutor) SendLine(str string) error {
	m.sendLine = append(m.sendLine, str)
	return nil
}

func (m *mockSessionExecutor) SendControl(char string) error {
	m.controls = append(m.controls, char)
	return nil
}

func TestSshNode_GetSSHCommand(t *testing.T) {
	t.Run("default command", func(t *testing.T) {
		n := NewSshNode("host1", "192.168.1.10", "admin", "pass", "ubuntu")
		cmd := n.GetSSHCommand()
		expected := "ssh admin@192.168.1.10"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})

	t.Run("with port and options", func(t *testing.T) {
		n := NewSshNode("host1", "192.168.1.10", "admin", "pass", "ubuntu")
		n.SSHPort = 2222
		n.SSHOptions = "-o StrictHostKeyChecking=no"
		cmd := n.GetSSHCommand()
		expected := "ssh -p 2222 admin@192.168.1.10 -o StrictHostKeyChecking=no"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})

	t.Run("with key and login flag", func(t *testing.T) {
		n := NewSshNode("host1", "192.168.1.10", "admin", "pass", "ubuntu").
			WithKey("./ssh/id_rsa.key").
			WithLoginFlag(true)
		cmd := n.GetSSHCommand()
		expected := "ssh -i ./ssh/id_rsa.key 192.168.1.10 -l admin"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})

	t.Run("with custom command func", func(t *testing.T) {
		n := NewSshNode("host1", "192.168.1.10", "admin", "pass", "ubuntu").
			WithCommandFunc(func(n *SshNode) string {
				return fmt.Sprintf("ssh %s -l %s -i ./ssh/id_rsa.key", n.AccessIP, n.Username)
			})
		cmd := n.GetSSHCommand()
		expected := "ssh 192.168.1.10 -l admin -i ./ssh/id_rsa.key"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})
}

func TestTelnetNode_GetTelnetCommand(t *testing.T) {
	t.Run("default command", func(t *testing.T) {
		n := NewTelnetNode("switch1", "10.0.0.1", "admin", "pass", "cisco")
		cmd := n.GetTelnetCommand()
		expected := "telnet 10.0.0.1"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})

	t.Run("custom port", func(t *testing.T) {
		n := NewTelnetNode("switch1", "10.0.0.1", "admin", "pass", "cisco")
		n.TelnetPort = 2323
		cmd := n.GetTelnetCommand()
		expected := "telnet 10.0.0.1 2323"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})

	t.Run("with command func", func(t *testing.T) {
		n := NewTelnetNode("switch1", "10.0.0.1", "admin", "pass", "cisco").
			WithCommandFunc(func(n *TelnetNode) string {
				return fmt.Sprintf("telnet -8 %s %d", n.AccessIP, n.TelnetPort)
			})
		n.TelnetPort = 23
		cmd := n.GetTelnetCommand()
		expected := "telnet -8 10.0.0.1 23"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})
}

func TestTelnetNode_EscapeExit(t *testing.T) {
	n := NewTelnetNode("switch1", "10.0.0.1", "admin", "pass", "cisco").
		WithEscapeExit()

	if n.GetExitHandler() == nil {
		t.Fatalf("expected ExitHandler to be set")
	}

	mockSess := &mockSessionExecutor{}
	err := n.GetExitHandler()(mockSess, n)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockSess.controls) != 1 || mockSess.controls[0] != "]" {
		t.Errorf("expected SendControl(']'), got: %v", mockSess.controls)
	}
	if len(mockSess.sendLine) != 1 || mockSess.sendLine[0] != "quit" {
		t.Errorf("expected SendLine('quit'), got: %v", mockSess.sendLine)
	}
}

func TestNodeHandlers(t *testing.T) {
	calledConnect := false
	calledExit := false

	n := NewSshNode("host1", "192.168.1.10", "admin", "pass", "ubuntu").
		WithConnectHandler(func(s SessionExecutor, n Node) error {
			calledConnect = true
			return nil
		}).
		WithExitHandler(func(s SessionExecutor, n Node) error {
			calledExit = true
			return nil
		})

	mockSess := &mockSessionExecutor{}
	if err := n.GetConnectHandler()(mockSess, n); err != nil {
		t.Fatalf("ConnectHandler error: %v", err)
	}
	if !calledConnect {
		t.Errorf("expected ConnectHandler to be called")
	}

	if err := n.GetExitHandler()(mockSess, n); err != nil {
		t.Fatalf("ExitHandler error: %v", err)
	}
	if !calledExit {
		t.Errorf("expected ExitHandler to be called")
	}
}

func TestNode_DirectMode(t *testing.T) {
	t.Run("BaseNode DirectMode", func(t *testing.T) {
		base := &BaseNode{
			ExitCommand:         "exit",
			InteractPreCommand:  "echo pre",
			InteractPostCommand: "echo post",
		}
		if base.IsDirectMode() {
			t.Errorf("default DirectMode should be false")
		}
		if base.GetExitCommand() != "exit" {
			t.Errorf("expected exit, got %s", base.GetExitCommand())
		}
		if base.GetInteractPreCommand() != "echo pre" {
			t.Errorf("expected echo pre, got %s", base.GetInteractPreCommand())
		}

		base.WithDirectMode(true)
		if !base.IsDirectMode() {
			t.Errorf("DirectMode should be true")
		}
		if base.GetExitCommand() != "" {
			t.Errorf("DirectMode GetExitCommand should be empty, got %s", base.GetExitCommand())
		}
		if base.GetInteractPreCommand() != "" {
			t.Errorf("DirectMode GetInteractPreCommand should be empty, got %s", base.GetInteractPreCommand())
		}
		if base.GetInteractPostCommand() != "" {
			t.Errorf("DirectMode GetInteractPostCommand should be empty, got %s", base.GetInteractPostCommand())
		}
	})

	t.Run("SshNode DirectMode", func(t *testing.T) {
		sshNode := NewSshNode("host1", "192.168.1.1", "user", "pass", "ubuntu")
		if sshNode.GetInteractPreCommand() != "stty echo; stty sane" {
			t.Errorf("expected default stty echo, got %s", sshNode.GetInteractPreCommand())
		}
		if sshNode.GetExitCommand() != "exit" {
			t.Errorf("expected exit, got %s", sshNode.GetExitCommand())
		}

		sshNode.WithDirectMode(true)
		if !sshNode.IsDirectMode() {
			t.Errorf("DirectMode should be true")
		}
		if sshNode.GetInteractPreCommand() != "" {
			t.Errorf("expected empty pre command, got %s", sshNode.GetInteractPreCommand())
		}
		if sshNode.GetInteractPostCommand() != "" {
			t.Errorf("expected empty post command, got %s", sshNode.GetInteractPostCommand())
		}
		if sshNode.GetExitCommand() != "" {
			t.Errorf("expected empty exit command, got %s", sshNode.GetExitCommand())
		}
	})

	t.Run("TelnetNode DirectMode", func(t *testing.T) {
		telnetNode := NewTelnetNode("host1", "192.168.1.1", "user", "pass", "cisco").
			WithDirectMode(true)
		if !telnetNode.IsDirectMode() {
			t.Errorf("DirectMode should be true")
		}
		if telnetNode.GetExitCommand() != "" {
			t.Errorf("expected empty exit command, got %s", telnetNode.GetExitCommand())
		}
	})
}
