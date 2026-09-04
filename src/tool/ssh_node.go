package tool

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"regexp"
	"strings"

	"golang.org/x/term"

	"goplur/src/node"
)

// SshNodeConfig holds the parameters required to initialize an SSH node.
type SshNodeConfig struct {
	Hostname string `json:"hostname" toml:"hostname"`
	AccessIP string `json:"access_ip" toml:"access_ip"`
	Username string `json:"username" toml:"username"`
	Password string `json:"password" toml:"password"`
	Platform string `json:"platform" toml:"platform"`
}

// Validate checks all fields of SshNodeConfig for correctness, ensuring no required fields are empty.
func (c *SshNodeConfig) Validate() error {
	if err := ValidateHostname(c.Hostname); err != nil {
		return err
	}
	if err := ValidateAccessIP(c.AccessIP); err != nil {
		return err
	}
	if err := ValidateUsername(c.Username); err != nil {
		return err
	}
	if err := ValidatePassword(c.Password); err != nil {
		return err
	}
	if err := ValidatePlatform(c.Platform); err != nil {
		return err
	}
	return nil
}

// ValidatePartial checks only the non-empty fields in SshNodeConfig.
// Useful for validating partial configurations loaded from JSON or TOML.
func (c *SshNodeConfig) ValidatePartial() error {
	if c.Hostname != "" {
		if err := ValidateHostname(c.Hostname); err != nil {
			return err
		}
	}
	if c.AccessIP != "" {
		if err := ValidateAccessIP(c.AccessIP); err != nil {
			return err
		}
	}
	if c.Username != "" {
		if err := ValidateUsername(c.Username); err != nil {
			return err
		}
	}
	if c.Password != "" {
		if err := ValidatePassword(c.Password); err != nil {
			return err
		}
	}
	if c.Platform != "" {
		if err := ValidatePlatform(c.Platform); err != nil {
			return err
		}
	}
	return nil
}

// ToNode converts SshNodeConfig to a *node.SshNode.
func (c *SshNodeConfig) ToNode() *node.SshNode {
	return node.NewSshNode(c.Hostname, c.AccessIP, c.Username, c.Password, c.Platform)
}

// DefaultSshNodeConfig returns system defaults for interactive configuration.
func DefaultSshNodeConfig() *SshNodeConfig {
	defaultHost, _ := os.Hostname()
	if idx := strings.Index(defaultHost, "."); idx != -1 {
		defaultHost = defaultHost[:idx]
	}
	if defaultHost == "" {
		defaultHost = "localhost"
	}

	defaultUser := os.Getenv("USER")
	if defaultUser == "" {
		if u, err := user.Current(); err == nil && u.Username != "" {
			defaultUser = u.Username
		}
	}
	if defaultUser == "" {
		defaultUser = "root"
	}

	defaultPlatform := node.DetectPlatform()
	if defaultPlatform == "" {
		defaultPlatform = "ubuntu"
	}

	return &SshNodeConfig{
		Hostname: defaultHost,
		AccessIP: "127.0.0.1",
		Username: defaultUser,
		Password: "",
		Platform: defaultPlatform,
	}
}

// MergeWithDefaults merges base config with override values (only non-empty fields are overwritten).
func MergeWithDefaults(base *SshNodeConfig, overrides *SshNodeConfig) *SshNodeConfig {
	result := &SshNodeConfig{}
	if base != nil {
		*result = *base
	}
	if overrides == nil {
		return result
	}
	if overrides.Hostname != "" {
		result.Hostname = overrides.Hostname
	}
	if overrides.AccessIP != "" {
		result.AccessIP = overrides.AccessIP
	}
	if overrides.Username != "" {
		result.Username = overrides.Username
	}
	if overrides.Password != "" {
		result.Password = overrides.Password
	}
	if overrides.Platform != "" {
		result.Platform = overrides.Platform
	}
	return result
}

// ParseSshNodeConfigJSON parses JSON byte slice into an SshNodeConfig.
func ParseSshNodeConfigJSON(data []byte) (*SshNodeConfig, error) {
	var cfg SshNodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return &cfg, nil
}

// ParseSshNodeConfigTOML parses TOML byte slice into an SshNodeConfig.
// Supports standard key = "value" pairs and [section] headers.
func ParseSshNodeConfigTOML(data []byte) (*SshNodeConfig, error) {
	cfg := &SshNodeConfig{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Strip comments if inline
		if idx := strings.Index(val, "#"); idx != -1 {
			val = strings.TrimSpace(val[:idx])
		}
		// Strip quotes
		val = strings.Trim(val, `"'`)
		switch strings.ToLower(key) {
		case "hostname", "host":
			cfg.Hostname = val
		case "access_ip", "accessip", "ip":
			cfg.AccessIP = val
		case "username", "user":
			cfg.Username = val
		case "password", "pass":
			cfg.Password = val
		case "platform":
			cfg.Platform = val
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}
	return cfg, nil
}

// LoadDefaultsFromJSON parses JSON, validates partial fields, and merges with system defaults.
func LoadDefaultsFromJSON(data []byte) (*SshNodeConfig, error) {
	raw, err := ParseSshNodeConfigJSON(data)
	if err != nil {
		return nil, err
	}
	if err := raw.ValidatePartial(); err != nil {
		return nil, fmt.Errorf("validation failed on JSON config: %w", err)
	}
	merged := MergeWithDefaults(DefaultSshNodeConfig(), raw)
	if err := merged.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed on merged config: %w", err)
	}
	return merged, nil
}

// LoadDefaultsFromTOML parses TOML, validates partial fields, and merges with system defaults.
func LoadDefaultsFromTOML(data []byte) (*SshNodeConfig, error) {
	raw, err := ParseSshNodeConfigTOML(data)
	if err != nil {
		return nil, err
	}
	if err := raw.ValidatePartial(); err != nil {
		return nil, fmt.Errorf("validation failed on TOML config: %w", err)
	}
	merged := MergeWithDefaults(DefaultSshNodeConfig(), raw)
	if err := merged.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed on merged config: %w", err)
	}
	return merged, nil
}

// ValidateHostname verifies that the hostname is non-empty, of valid length,
// and consists of allowed hostname characters.
func ValidateHostname(hostname string) error {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if len(hostname) > 253 {
		return fmt.Errorf("hostname exceeds maximum length of 253 characters")
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]([a-zA-Z0-9\-_.]*[a-zA-Z0-9])?$`, hostname)
	if !matched {
		return fmt.Errorf("hostname %q contains invalid characters (allowed: alphanumeric, hyphen, dot, underscore)", hostname)
	}
	return nil
}

// ValidateAccessIP verifies that accessIP is a valid IPv4/IPv6 address or resolvable hostname.
func ValidateAccessIP(accessIP string) error {
	accessIP = strings.TrimSpace(accessIP)
	if accessIP == "" {
		return fmt.Errorf("access IP cannot be empty")
	}
	if net.ParseIP(accessIP) != nil {
		return nil
	}
	if err := ValidateHostname(accessIP); err == nil {
		return nil
	}
	return fmt.Errorf("invalid access IP or hostname: %q", accessIP)
}

// ValidateUsername verifies that username is non-empty and contains valid Unix username characters.
func ValidateUsername(username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if len(username) > 32 {
		return fmt.Errorf("username exceeds maximum length of 32 characters")
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_\-]+$`, username)
	if !matched {
		return fmt.Errorf("username %q contains invalid characters (allowed: alphanumeric, underscore, hyphen)", username)
	}
	return nil
}

// ValidatePassword validates password input (empty password is valid for key-based authentication).
func ValidatePassword(password string) error {
	return nil
}

// ValidatePlatform verifies that platform matches supported Linux distributions in goplur.
func ValidatePlatform(platform string) error {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return fmt.Errorf("platform cannot be empty")
	}
	lower := strings.ToLower(platform)
	supported := []string{
		"ubuntu", "debian", "centos", "almalinux", "alma",
		"rocky", "rhel", "fedora", "arch", "linux",
	}
	for _, s := range supported {
		if strings.Contains(lower, s) {
			return nil
		}
	}
	return fmt.Errorf("unsupported platform %q (supported: ubuntu, debian, centos, almalinux, rocky, rhel, fedora, arch)", platform)
}

func promptField(reader *bufio.Reader, w io.Writer, label string, defaultValue string, validate func(string) error) (string, error) {
	for {
		if defaultValue != "" {
			fmt.Fprintf(w, "Enter %s [default: %s]: ", label, defaultValue)
		} else {
			fmt.Fprintf(w, "Enter %s: ", label)
		}

		line, err := reader.ReadString('\n')
		val := strings.TrimSpace(line)
		if val == "" {
			val = defaultValue
		}

		if validate != nil {
			if vErr := validate(val); vErr != nil {
				fmt.Fprintf(w, "Error: %v. Please try again.\n", vErr)
				if err == io.EOF {
					return "", vErr
				}
				continue
			}
		}
		if err != nil && err != io.EOF {
			return "", err
		}
		return val, nil
	}
}

func promptPassword(r io.Reader, reader *bufio.Reader, w io.Writer, label string, validate func(string) error) (string, error) {
	for {
		fmt.Fprintf(w, "Enter %s (leave empty for key-based auth): ", label)

		var val string
		var readErr error

		if f, ok := r.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			passBytes, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(w)
			if err != nil {
				return "", err
			}
			val = string(passBytes)
		} else {
			line, err := reader.ReadString('\n')
			val = strings.TrimRight(line, "\r\n")
			readErr = err
		}

		if validate != nil {
			if vErr := validate(val); vErr != nil {
				fmt.Fprintf(w, "Error: %v. Please try again.\n", vErr)
				if readErr == io.EOF {
					return "", vErr
				}
				continue
			}
		}
		if readErr != nil && readErr != io.EOF {
			return "", readErr
		}
		return val, nil
	}
}

// PromptSshNodeConfigWithDefaults interactively prompts for SSH node parameters,
// using the given defaults (or system defaults if defaults is nil).
func PromptSshNodeConfigWithDefaults(r io.Reader, w io.Writer, defaults *SshNodeConfig) (*SshNodeConfig, error) {
	reader := bufio.NewReader(r)

	baseDefaults := DefaultSshNodeConfig()
	if defaults != nil {
		baseDefaults = MergeWithDefaults(baseDefaults, defaults)
	}

	hostname, err := promptField(reader, w, "target Hostname", baseDefaults.Hostname, ValidateHostname)
	if err != nil {
		return nil, err
	}

	accessIP, err := promptField(reader, w, "target Access IP", baseDefaults.AccessIP, ValidateAccessIP)
	if err != nil {
		return nil, err
	}

	username, err := promptField(reader, w, "Username", baseDefaults.Username, ValidateUsername)
	if err != nil {
		return nil, err
	}

	password, err := promptPassword(r, reader, w, "Password", ValidatePassword)
	if err != nil {
		return nil, err
	}
	if password == "" && baseDefaults.Password != "" {
		password = baseDefaults.Password
	}

	platform, err := promptField(reader, w, "Platform", baseDefaults.Platform, ValidatePlatform)
	if err != nil {
		return nil, err
	}

	return &SshNodeConfig{
		Hostname: hostname,
		AccessIP: accessIP,
		Username: username,
		Password: password,
		Platform: platform,
	}, nil
}

// PromptSshNodeConfig interactively prompts for SSH node parameters using system defaults.
func PromptSshNodeConfig(r io.Reader, w io.Writer) (*SshNodeConfig, error) {
	return PromptSshNodeConfigWithDefaults(r, w, nil)
}

// PromptSshNodeWithDefaults interactively prompts for SSH node parameters using custom defaults,
// returning an initialized *node.SshNode.
func PromptSshNodeWithDefaults(r io.Reader, w io.Writer, defaults *SshNodeConfig) (*node.SshNode, error) {
	cfg, err := PromptSshNodeConfigWithDefaults(r, w, defaults)
	if err != nil {
		return nil, err
	}
	return cfg.ToNode(), nil
}

// PromptSshNodeWithIO interactively prompts for SSH node parameters using custom reader and writer,
// returning an initialized *node.SshNode.
func PromptSshNodeWithIO(r io.Reader, w io.Writer) (*node.SshNode, error) {
	return PromptSshNodeWithDefaults(r, w, nil)
}

// PromptSshNode interactively prompts for SSH node parameters using standard input and output.
func PromptSshNode() (*node.SshNode, error) {
	return PromptSshNodeWithDefaults(os.Stdin, os.Stdout, nil)
}

// NewInteractiveSshNode is an alias for PromptSshNode.
func NewInteractiveSshNode() (*node.SshNode, error) {
	return PromptSshNode()
}
