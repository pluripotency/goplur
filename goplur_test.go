package goplur

import (
	"os"
	"strings"
	"testing"
)

func TestNode(t *testing.T) {
	node := NewMeNode()
	if node.Hostname == "" {
		t.Error("hostname should not be empty")
	}
	if node.Username == "" {
		t.Error("username should not be empty")
	}
	t.Logf("Hostname: %s, Username: %s, Platform: %s, WaitPrompt: %s",
		node.Hostname, node.Username, node.Platform, node.WaitPrompt)
}

func TestBashSession(t *testing.T) {
	logParams := LogParams{
		EnableStdout: false,
	}

	node := NewMeNode()
	s := NewSession(node, &logParams)
	defer s.Close()

	_, err := s.Bash()
	if err != nil {
		t.Fatalf("failed to start bash: %v", err)
	}

	out, err := s.Run("echo 'hello from goplur'")
	if err != nil {
		t.Fatalf("failed to run command: %v", err)
	}

	if !strings.Contains(out, "hello from goplur") {
		t.Errorf("expected output to contain hello message, got: %q", out)
	}
}

func TestShellOperations(t *testing.T) {
	logParams := LogParams{
		EnableStdout: false,
	}

	node := NewMeNode()
	s := NewSession(node, &logParams)
	defer s.Close()

	_, err := s.Bash()
	if err != nil {
		t.Fatalf("failed to start bash: %v", err)
	}

	testFile := "/tmp/goplur_test_file.txt"
	defer os.Remove(testFile)

	// Create and write file using HereDoc
	content := "line 1\nline 2\nline 3"
	err = s.HereDoc(testFile, content, "'EOF'")
	if err != nil {
		t.Fatalf("failed to write heredoc: %v", err)
	}

	// Verify file exists
	exists, err := s.CheckFileExists(testFile)
	if err != nil || !exists {
		t.Fatalf("expected file to exist, err: %v", err)
	}

	// Read and verify file contents
	out, err := s.Run("cat " + testFile)
	if err != nil {
		t.Fatalf("failed to run cat: %v", err)
	}
	if !strings.Contains(out, "line 2") {
		t.Errorf("expected cat output to contain 'line 2', got: %q", out)
	}

	// Test line check
	lineOk, err := s.CheckLineExistsInFile(testFile, "line 2")
	if err != nil || !lineOk {
		t.Errorf("expected line to exist, err: %v", err)
	}

	// Test SedReplace
	_, err = s.SedReplace("line 2", "replaced line", testFile, "")
	if err != nil {
		t.Fatalf("failed to replace line with sed: %v", err)
	}

	// Verify replacement
	out, err = s.Run("cat " + testFile)
	if err != nil {
		t.Fatalf("failed to run cat: %v", err)
	}
	if !strings.Contains(out, "replaced line") || strings.Contains(out, "line 2") {
		t.Errorf("expected output to contain replaced line, got: %q", out)
	}
}

func TestSessionWrap(t *testing.T) {
	logParams := LogParams{
		EnableStdout: false,
	}

	testFile := "/tmp/goplur_wrap_test.txt"
	defer os.Remove(testFile)

	err := RunBash(nil, &logParams, func(s *Session) error {
		_, err := s.Run("echo 'wrap test' > " + testFile)
		return err
	})
	if err != nil {
		t.Fatalf("RunBash wrapper failed: %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}
	if !strings.Contains(string(data), "wrap test") {
		t.Errorf("expected file to contain wrap test, got %q", string(data))
	}
}

func TestNewNodeTypes(t *testing.T) {
	// Test SshNode
	sshNode := NewSshNode("ssh-host", "10.0.0.1", "ssh-user", "ssh-pass", "ubuntu")
	if sshNode.GetHostname() != "ssh-host" {
		t.Errorf("expected ssh hostname 'ssh-host', got %q", sshNode.GetHostname())
	}
	if sshNode.GetAccessIP() != "10.0.0.1" {
		t.Errorf("expected ssh access ip '10.0.0.1', got %q", sshNode.GetAccessIP())
	}
	if sshNode.SSHPort != 22 {
		t.Errorf("expected default ssh port 22, got %d", sshNode.SSHPort)
	}
	if sshNode.GetExitCommand() != "exit" {
		t.Errorf("expected default exit command 'exit', got %q", sshNode.GetExitCommand())
	}

	// Test TelnetNode
	telnetNode := NewTelnetNode("telnet-host", "10.0.0.2", "telnet-user", "telnet-pass", "centos7")
	if telnetNode.GetHostname() != "telnet-host" {
		t.Errorf("expected telnet hostname 'telnet-host', got %q", telnetNode.GetHostname())
	}
	if telnetNode.TelnetPort != 23 {
		t.Errorf("expected default telnet port 23, got %d", telnetNode.TelnetPort)
	}

	// Test custom values
	sshNode.SSHPort = 2222
	if sshNode.GetSSHPort() != 2222 {
		t.Errorf("expected custom ssh port 2222, got %d", sshNode.GetSSHPort())
	}

	telnetNode.TelnetPort = 2323
	if telnetNode.GetTelnetPort() != 2323 {
		t.Errorf("expected custom telnet port 2323, got %d", telnetNode.GetTelnetPort())
	}
}
