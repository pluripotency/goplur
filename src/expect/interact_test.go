package expect

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestInteractWithIO(t *testing.T) {
	// Spawn a process using SpawnWithArgs so arguments are not split by whitespace
	e, _, err := SpawnWithArgs([]string{"bash", "-c", "read line; echo RECV:$line"}, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to spawn: %v", err)
	}
	defer e.Close()

	var inBuf bytes.Buffer
	inBuf.WriteString("hello from interact\n")

	var outBuf bytes.Buffer
	err = e.InteractWithIO(&inBuf, &outBuf)
	if err != nil {
		t.Fatalf("InteractWithIO returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	outStr := e.out.String()
	if !strings.Contains(outStr, "RECV:hello from interact") {
		t.Errorf("expected output to contain 'RECV:hello from interact', got: %q", outStr)
	}
}
