package session

import (
	"fmt"
	nd "goplur/src/node"
	"testing"
)

func TestGetControlChar(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a", "\x01"},
		{"c", "\x03"},
		{"z", "\x1a"},
		{"A", "\x01"},
		{"C", "\x03"},
		{"Z", "\x1a"},
		{"@", "\x00"},
		{"[", "\x1b"},
		{"\\", "\x1c"},
		{"]", "\x1d"},
		{"^", "\x1e"},
		{"_", "\x1f"},
		{"?", "\x7f"},
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input_%s", tt.input), func(t *testing.T) {
			res := getControlChar(tt.input)
			if res != tt.expected {
				t.Errorf("input %q: expected %q (len %d), got %q (len %d)", tt.input, tt.expected, len(tt.expected), res, len(res))
			}
		})
	}
}

func TestRunSession_CustomExitHandler(t *testing.T) {
	bashNode := nd.NewMeNode()
	exitHandlerCalled := false

	bashNode.ExitHandler = func(s nd.SessionExecutor, n nd.Node) error {
		exitHandlerCalled = true
		return s.SendLine("exit")
	}

	err := RunSession(bashNode, "bash", nil, func(s *Session) error {
		out, err := s.Run("echo hello_custom_exit")
		if err != nil {
			return err
		}
		if out == "" {
			t.Errorf("expected non-empty output")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("RunSession failed: %v", err)
	}

	if !exitHandlerCalled {
		t.Errorf("expected custom ExitHandler to be called")
	}
}

func TestRunSession_CustomConnectHandler(t *testing.T) {
	bashNode := nd.NewMeNode()
	connectHandlerCalled := false

	bashNode.ConnectHandler = func(s nd.SessionExecutor, n nd.Node) error {
		connectHandlerCalled = true
		sess := s.(*Session)
		_, err := sess.Do("bash", []ExpectRow{
			{Pattern: "", Reaction: ReactionCapture},
		}, sess.timeout)
		return err
	}

	err := RunSession(bashNode, "bash", nil, func(s *Session) error {
		out, err := s.Run("echo hello_custom_connect")
		if err != nil {
			return err
		}
		if out == "" {
			t.Errorf("expected non-empty output")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("RunSession failed: %v", err)
	}

	if !connectHandlerCalled {
		t.Errorf("expected custom ConnectHandler to be called")
	}
}

func TestSessionSendMethods(t *testing.T) {
	bashNode := nd.NewMeNode()

	err := RunSession(bashNode, "bash", nil, func(s *Session) error {
		// SendLine でコマンドを送り、プロンプトを待つ
		err := s.SendLine("echo raw_send_line")
		if err != nil {
			return err
		}

		res, err := s.Do("", []ExpectRow{
			{Pattern: `raw_send_line`, Reaction: ReactionSuccess, Arg: true},
		}, s.timeout)
		if err != nil {
			return err
		}
		if res != true {
			t.Errorf("expected true from Do reaction")
		}

		// SendControl のエラーケース（未対応文字）
		err = s.SendControl("invalid")
		if err == nil {
			t.Errorf("expected error for invalid control character")
		}

		return nil
	})

	if err != nil {
		t.Fatalf("RunSession failed: %v", err)
	}
}
