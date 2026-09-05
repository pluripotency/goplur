package session

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"goplur/src/node"
)

func TestSessionInteractWithIO(t *testing.T) {
	n := node.NewMeNode()
	logParams := LogParams{EnableStdout: false}
	s := NewSession(n, &logParams)
	defer s.Close()

	_, err := s.Bash()
	if err != nil {
		t.Fatalf("failed to start bash: %v", err)
	}

	var inBuf bytes.Buffer
	inBuf.WriteString("echo 'hello from session interact'\nexit\n")

	var outBuf bytes.Buffer
	err = s.InteractWithIO(&inBuf, &outBuf)
	if err != nil {
		t.Fatalf("InteractWithIO error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	outStr := s.child.Output()
	if !strings.Contains(outStr, "hello from session interact") {
		t.Errorf("expected output to contain 'hello from session interact', got: %q", outStr)
	}
}

func TestResolveInteractConfig_Defaults(t *testing.T) {
	// SshNode defaults
	sshNode := node.NewSshNode("testhost", "192.168.1.10", "testuser", "pass", "almalinux9")
	s1 := NewSession(sshNode, &LogParams{EnableStdout: false})
	cfg1 := s1.resolveInteractConfig()
	if cfg1.PreCommand == nil || *cfg1.PreCommand != "stty echo; stty sane" {
		t.Errorf("expected SshNode pre-command 'stty echo; stty sane', got: %v", cfg1.PreCommand)
	}
	if cfg1.PostCommand == nil || *cfg1.PostCommand != "stty -echo" {
		t.Errorf("expected SshNode post-command 'stty -echo', got: %v", cfg1.PostCommand)
	}

	// BashNode defaults
	bashNode := node.NewMeNode()
	s2 := NewSession(bashNode, &LogParams{EnableStdout: false})
	cfg2 := s2.resolveInteractConfig()
	if cfg2.PreCommand == nil || *cfg2.PreCommand != "stty echo" {
		t.Errorf("expected BashNode pre-command 'stty echo', got: %v", cfg2.PreCommand)
	}

	// BaseNode defaults (raw console / unconfigured)
	baseNode := &node.BaseNode{Hostname: "raw"}
	s3 := NewSession(baseNode, &LogParams{EnableStdout: false})
	cfg3 := s3.resolveInteractConfig()
	if cfg3.PreCommand == nil || *cfg3.PreCommand != "" {
		t.Errorf("expected BaseNode pre-command '', got: %v", cfg3.PreCommand)
	}
}

func TestResolveInteractConfig_OptionsOverride(t *testing.T) {
	sshNode := node.NewSshNode("testhost", "192.168.1.10", "testuser", "pass", "almalinux9")
	s := NewSession(sshNode, &LogParams{EnableStdout: false})

	// Override pre-command with "reset"
	cfg1 := s.resolveInteractConfig(WithPreCommand("reset"))
	if cfg1.PreCommand == nil || *cfg1.PreCommand != "reset" {
		t.Errorf("expected pre-command 'reset', got: %v", cfg1.PreCommand)
	}

	// Suppress all commands with WithoutCommands()
	cfg2 := s.resolveInteractConfig(WithoutCommands())
	if cfg2.PreCommand == nil || *cfg2.PreCommand != "" {
		t.Errorf("expected empty pre-command, got: %v", cfg2.PreCommand)
	}
	if cfg2.PostCommand == nil || *cfg2.PostCommand != "" {
		t.Errorf("expected empty post-command, got: %v", cfg2.PostCommand)
	}

	// PreHook and PostHook
	hookCalled := false
	cfg3 := s.resolveInteractConfig(WithPreHook(func(sess *Session) error {
		hookCalled = true
		return nil
	}))
	if cfg3.PreHook == nil {
		t.Fatalf("expected PreHook to be set")
	}
	_ = cfg3.PreHook(s)
	if !hookCalled {
		t.Errorf("expected PreHook to have been invoked")
	}
}

func TestSessionInteract_WithWithoutCommands(t *testing.T) {
	n := node.NewMeNode()
	logParams := LogParams{EnableStdout: false}
	s := NewSession(n, &logParams)
	defer s.Close()

	_, err := s.Bash()
	if err != nil {
		t.Fatalf("failed to start bash: %v", err)
	}

	preHookRun := false
	var inBuf bytes.Buffer
	inBuf.WriteString("echo 'hello without commands'\nexit\n")

	var outBuf bytes.Buffer
	err = s.InteractWithIO(&inBuf, &outBuf,
		WithoutCommands(),
		WithPreHook(func(sess *Session) error {
			preHookRun = true
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("InteractWithIO error: %v", err)
	}

	if !preHookRun {
		t.Errorf("expected preHook to run")
	}

	time.Sleep(50 * time.Millisecond)

	outStr := s.child.Output()
	if !strings.Contains(outStr, "hello without commands") {
		t.Errorf("expected output to contain 'hello without commands', got: %q", outStr)
	}
}
