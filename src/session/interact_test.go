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
