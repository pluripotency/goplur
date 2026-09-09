package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	nd "goplur/src/node"
)

// createMockScript creates a temporary shell script that mimics remote host SSH behavior.
func createMockScript(t *testing.T, scriptContent string) string {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "mock_ssh.sh")
	content := "#!/bin/bash\n" + scriptContent
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write mock script: %v", err)
	}
	return scriptPath
}

// TestDirectLogin_FastPath_FortiGate verifies that a FortiGate prompt is immediately detected via fast-path.
func TestDirectLogin_FastPath_FortiGate(t *testing.T) {
	script := createMockScript(t, `
echo "FortiGate-60F (global) # "
sleep 5
`)

	node := nd.NewSshNode("fortigate", "192.168.1.99", "admin", "secret", "fortinet").
		WithDirectMode(true).
		WithCommandFunc(func(n *nd.SshNode) string {
			return script
		})

	start := time.Now()
	err := RunSession(node, "ssh", nil, func(s *Session) error {
		// Connected successfully
		return nil
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected direct login to succeed, got: %v", err)
	}

	// Should be very fast (well under 2 seconds) due to fast-path regex
	if elapsed > 1500*time.Millisecond {
		t.Errorf("expected fast-path login to complete quickly, took %v", elapsed)
	}
}

// TestDirectLogin_PasswordAuth verifies that password prompt is handled and followed by prompt detection.
func TestDirectLogin_PasswordAuth(t *testing.T) {
	script := createMockScript(t, `
echo -n "Password: "
read -r pass
if [ "$pass" = "secret" ]; then
    echo "Welcome to Appliance"
    echo "appliance> "
    sleep 5
else
    echo "Permission denied, please try again."
    exit 1
fi
`)

	node := nd.NewSshNode("appliance", "192.168.1.100", "admin", "secret", "generic").
		WithDirectMode(true).
		WithCommandFunc(func(n *nd.SshNode) string {
			return script
		})

	err := RunSession(node, "ssh", nil, func(s *Session) error {
		return nil
	})

	if err != nil {
		t.Fatalf("expected password login to succeed, got: %v", err)
	}
}

// TestDirectLogin_DelayedHost verifies that initial silence (e.g. DNS reverse lookup delay)
// does not trigger false positive key-auth, and correctly waits for password prompt.
func TestDirectLogin_DelayedHost(t *testing.T) {
	script := createMockScript(t, `
# Simulate 2.5 seconds of silence (DNS reverse lookup delay)
sleep 2.5
echo -n "Password: "
read -r pass
if [ "$pass" = "secret" ]; then
    echo "Linux myhost 5.15.0"
    echo "user@myhost:~$ "
    sleep 5
else
    echo "Permission denied."
    exit 1
fi
`)

	node := nd.NewSshNode("slowhost", "10.0.0.5", "user", "secret", "ubuntu").
		WithDirectMode(true).
		WithCommandFunc(func(n *nd.SshNode) string {
			return script
		})

	start := time.Now()
	err := RunSession(node, "ssh", nil, func(s *Session) error {
		return nil
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected delayed host login to succeed, got: %v", err)
	}

	// Elapsed time should be >= 2.5 seconds
	if elapsed < 2500*time.Millisecond {
		t.Errorf("expected elapsed >= 2.5s due to delay, got %v", elapsed)
	}
}

// TestDirectLogin_AtenMenu verifies that a menu screen without traditional prompt symbols
// is safely detected by the interval output presence check.
func TestDirectLogin_AtenMenu(t *testing.T) {
	script := createMockScript(t, `
echo "=== ATEN KVM Switch Menu ==="
echo "1. Port 1 - Web01"
echo "2. Port 2 - DB01"
echo "Please type a number"
sleep 5
`)

	node := nd.NewSshNode("aten-kvm", "192.168.1.200", "admin", "secret", "aten").
		WithDirectMode(true).
		WithCommandFunc(func(n *nd.SshNode) string {
			return script
		})

	start := time.Now()
	err := RunSession(node, "ssh", nil, func(s *Session) error {
		return nil
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected ATEN menu detection to succeed, got: %v", err)
	}

	// ATEN menu has no trailing prompt symbol, so it should be detected at the 2-second interval mark
	if elapsed < 1800*time.Millisecond {
		t.Errorf("expected interval check (~2s), completed in %v", elapsed)
	}
}

// TestDirectLogin_NoResponseTimeout verifies that if host stays silent throughout all retries,
// it properly times out with an error.
func TestDirectLogin_NoResponseTimeout(t *testing.T) {
	script := createMockScript(t, `
# Totally silent for 15 seconds
sleep 15
`)

	node := nd.NewSshNode("deadhost", "10.0.0.99", "admin", "secret", "generic").
		WithDirectMode(true).
		WithCommandFunc(func(n *nd.SshNode) string {
			return script
		})

	start := time.Now()
	err := RunSession(node, "ssh", nil, func(s *Session) error {
		return nil
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected timeout error for completely silent host, got nil")
	}

	// Should time out around 8 seconds (2s * 4)
	if elapsed < 7*time.Second {
		t.Errorf("expected timeout after ~8s, took %v: %v", elapsed, err)
	}
	fmt.Printf("Dead host timeout correctly fired in %v with error: %v\n", elapsed, err)
}
