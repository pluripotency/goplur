package tool

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidation(t *testing.T) {
	// Hostname validation
	if err := ValidateHostname("localhost"); err != nil {
		t.Errorf("expected localhost to be valid, got: %v", err)
	}
	if err := ValidateHostname("web-server-01.prod.local"); err != nil {
		t.Errorf("expected valid hostname, got: %v", err)
	}
	if err := ValidateHostname(""); err == nil {
		t.Error("expected error for empty hostname")
	}
	if err := ValidateHostname("bad hostname with spaces"); err == nil {
		t.Error("expected error for hostname with spaces")
	}

	// AccessIP validation
	if err := ValidateAccessIP("127.0.0.1"); err != nil {
		t.Errorf("expected 127.0.0.1 to be valid, got: %v", err)
	}
	if err := ValidateAccessIP("::1"); err != nil {
		t.Errorf("expected ::1 to be valid, got: %v", err)
	}
	if err := ValidateAccessIP("localhost"); err != nil {
		t.Errorf("expected localhost to be valid, got: %v", err)
	}
	if err := ValidateAccessIP(""); err == nil {
		t.Error("expected error for empty access IP")
	}
	if err := ValidateAccessIP("bad ip with spaces"); err == nil {
		t.Error("expected error for invalid IP")
	}

	// Username validation
	if err := ValidateUsername("worker"); err != nil {
		t.Errorf("expected worker to be valid, got: %v", err)
	}
	if err := ValidateUsername("app_user-01"); err != nil {
		t.Errorf("expected app_user-01 to be valid, got: %v", err)
	}
	if err := ValidateUsername(""); err == nil {
		t.Error("expected error for empty username")
	}
	if err := ValidateUsername("user with space"); err == nil {
		t.Error("expected error for username with space")
	}

	// Platform validation
	if err := ValidatePlatform("ubuntu"); err != nil {
		t.Errorf("expected ubuntu to be valid, got: %v", err)
	}
	if err := ValidatePlatform("almalinux9"); err != nil {
		t.Errorf("expected almalinux9 to be valid, got: %v", err)
	}
	if err := ValidatePlatform("centos7"); err != nil {
		t.Errorf("expected centos7 to be valid, got: %v", err)
	}
	if err := ValidatePlatform(""); err == nil {
		t.Error("expected error for empty platform")
	}
	if err := ValidatePlatform("windows"); err == nil {
		t.Error("expected error for unsupported platform")
	}
}

func TestParseJSONAndTOML(t *testing.T) {
	jsonStr := `{
		"hostname": "json-host",
		"access_ip": "10.10.10.1",
		"username": "json-user",
		"password": "json-pass",
		"platform": "ubuntu"
	}`

	cfgJSON, err := ParseSshNodeConfigJSON([]byte(jsonStr))
	if err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if cfgJSON.Hostname != "json-host" || cfgJSON.AccessIP != "10.10.10.1" || cfgJSON.Username != "json-user" {
		t.Errorf("unexpected values from JSON: %+v", cfgJSON)
	}
	if err := cfgJSON.Validate(); err != nil {
		t.Errorf("expected valid JSON config, got: %v", err)
	}

	tomlStr := `
# Node configuration
hostname = "toml-host"
access_ip = '10.20.30.40'
username = "toml-user"
password = "toml-pass"
platform = "almalinux9"
`

	cfgTOML, err := ParseSshNodeConfigTOML([]byte(tomlStr))
	if err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}
	if cfgTOML.Hostname != "toml-host" || cfgTOML.AccessIP != "10.20.30.40" || cfgTOML.Username != "toml-user" {
		t.Errorf("unexpected values from TOML: %+v", cfgTOML)
	}
	if err := cfgTOML.Validate(); err != nil {
		t.Errorf("expected valid TOML config, got: %v", err)
	}
}

func TestPartialConfigAndDefaults(t *testing.T) {
	// Partial JSON with missing keys (username, password omitted)
	partialJSON := `{"hostname": "my-server", "platform": "ubuntu"}`

	merged, err := LoadDefaultsFromJSON([]byte(partialJSON))
	if err != nil {
		t.Fatalf("unexpected error loading partial JSON: %v", err)
	}

	if merged.Hostname != "my-server" {
		t.Errorf("expected my-server, got: %s", merged.Hostname)
	}
	if merged.Platform != "ubuntu" {
		t.Errorf("expected ubuntu, got: %s", merged.Platform)
	}
	// Omitted keys should take interactive system defaults
	if merged.AccessIP != "127.0.0.1" {
		t.Errorf("expected default access IP 127.0.0.1, got: %s", merged.AccessIP)
	}
	if merged.Username == "" {
		t.Error("expected non-empty username from system defaults")
	}

	// Invalid value in partial JSON should fail validation
	invalidJSON := `{"hostname": "bad host with space"}`
	_, err = LoadDefaultsFromJSON([]byte(invalidJSON))
	if err == nil {
		t.Error("expected error for invalid hostname in JSON")
	}
}

func TestPartialTOMLAndDefaults(t *testing.T) {
	partialTOML := `
hostname = "toml-partial"
access_ip = "192.168.1.50"
`

	merged, err := LoadDefaultsFromTOML([]byte(partialTOML))
	if err != nil {
		t.Fatalf("unexpected error loading partial TOML: %v", err)
	}

	if merged.Hostname != "toml-partial" {
		t.Errorf("expected toml-partial, got: %s", merged.Hostname)
	}
	if merged.AccessIP != "192.168.1.50" {
		t.Errorf("expected 192.168.1.50, got: %s", merged.AccessIP)
	}
	if merged.Platform == "" {
		t.Error("expected non-empty platform from system defaults")
	}
}

func TestPromptSshNodeWithCustomDefaults(t *testing.T) {
	customDefaults := &SshNodeConfig{
		Hostname: "preset-host",
		AccessIP: "10.0.0.1",
		Username: "preset-user",
		Password: "preset-pass",
		Platform: "almalinux9",
	}

	// Press Enter for all prompts to accept custom defaults
	input := "\n\n\n\n\n"
	var inBuf bytes.Buffer
	inBuf.WriteString(input)
	var outBuf bytes.Buffer

	node, err := PromptSshNodeWithDefaults(&inBuf, &outBuf, customDefaults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.Hostname != "preset-host" {
		t.Errorf("expected preset-host, got: %s", node.Hostname)
	}
	if node.AccessIP != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got: %s", node.AccessIP)
	}
	if node.Username != "preset-user" {
		t.Errorf("expected preset-user, got: %s", node.Username)
	}
	if node.Password != "preset-pass" {
		t.Errorf("expected preset-pass, got: %s", node.Password)
	}
	if node.Platform != "almalinux9" {
		t.Errorf("expected almalinux9, got: %s", node.Platform)
	}
}

func TestPromptSshNodeWithDefaults(t *testing.T) {
	// Simulate user pressing Enter for all prompts
	input := "\n\n\n\n\n"
	var inBuf bytes.Buffer
	inBuf.WriteString(input)
	var outBuf bytes.Buffer

	node, err := PromptSshNodeWithIO(&inBuf, &outBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.Hostname == "" {
		t.Error("expected non-empty default hostname")
	}
	if node.AccessIP != "127.0.0.1" {
		t.Errorf("expected default access IP 127.0.0.1, got: %s", node.AccessIP)
	}
	if node.Username == "" {
		t.Error("expected non-empty default username")
	}
	if node.Password != "" {
		t.Errorf("expected default empty password, got: %s", node.Password)
	}
	if node.Platform == "" {
		t.Error("expected non-empty default platform")
	}
	if node.SSHPort != 22 {
		t.Errorf("expected default SSH port 22, got: %d", node.SSHPort)
	}
}

func TestPromptSshNodeCustomValues(t *testing.T) {
	input := strings.Join([]string{
		"custom-host",
		"192.168.1.100",
		"custom-user",
		"secretPass123",
		"almalinux9",
	}, "\n") + "\n"

	var inBuf bytes.Buffer
	inBuf.WriteString(input)
	var outBuf bytes.Buffer

	node, err := PromptSshNodeWithIO(&inBuf, &outBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.Hostname != "custom-host" {
		t.Errorf("expected custom-host, got: %s", node.Hostname)
	}
	if node.AccessIP != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got: %s", node.AccessIP)
	}
	if node.Username != "custom-user" {
		t.Errorf("expected custom-user, got: %s", node.Username)
	}
	if node.Password != "secretPass123" {
		t.Errorf("expected secretPass123, got: %s", node.Password)
	}
	if node.Platform != "almalinux9" {
		t.Errorf("expected almalinux9, got: %s", node.Platform)
	}
}

func TestPromptSshNodeRepromptOnInvalidInput(t *testing.T) {
	// First provide invalid hostname ("invalid space host"), then valid "myhost"
	input := strings.Join([]string{
		"invalid space host",
		"myhost",
		"10.0.0.5",
		"admin",
		"pass",
		"ubuntu",
	}, "\n") + "\n"

	var inBuf bytes.Buffer
	inBuf.WriteString(input)
	var outBuf bytes.Buffer

	node, err := PromptSshNodeWithIO(&inBuf, &outBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.Hostname != "myhost" {
		t.Errorf("expected myhost after reprompt, got: %s", node.Hostname)
	}
	if !strings.Contains(outBuf.String(), "Please try again") {
		t.Errorf("expected output to contain error/reprompt message, got:\n%s", outBuf.String())
	}
}
