[日本語版 (Japanese Version)](./README_jp.md)

# goplur

> [!NOTE]
> **Acknowledgment & Appreciation:**
> The core terminal interaction and expectation matching engine in `goplur` is built upon code extracted and simplified from the [google/goexpect](https://github.com/google/goexpect) library. We would like to express our deep gratitude to the original authors of `goexpect` for their wonderful work. The extracted subset is now inlined directly inside this project (`goplur/goexpect.go`), achieving near-instantaneous execution speed (e.g., resolving the default 2-second polling latency) and removing heavy external dependencies (such as gRPC and Go SSH client).

`goplur` is a Go-based CLI automation tool designed for managing interactive sessions and performing idempotent shell operations across various platforms. Originally built on top of `github.com/google/goexpect`, it is the Go version of the Python `plur` library.

It provides a high-level API for automating SSH, Telnet, and local Bash sessions, matching expectations, elevating privileges dynamically, and writing logs safely.

## Features

- **Session Management**: Easily manage nested sessions (SSH, Telnet, SU, SUDO) with a stack-based node architecture.
- **Interactive Automation**: Handle complex interactive prompts (such as SSH key confirmations and passwords) automatically using configurable expectation sequences.
- **Secure Logging**: Suppresses logs (toggles muting/unmuting stdout and file outputs) during password prompts to avoid exposing credentials.
- **Idempotent Operations**: Built-in helper methods for common administrative tasks (e.g., file edits using `sed`, package management with `yum`/`dnf`, backup creations, and `HereDoc` configurations).
- **Cross-Platform Support**: Automatically detects and adapts to various Linux environments (AlmaLinux, CentOS, Ubuntu, Arch Linux).

---

## Installation

Add `goplur` and its dependency to your Go project:

```bash
go get goplur
```

*(Alternatively, if importing as a remote package, use `go get github.com/pluripotency/goplur`)*

---

## Basic Usage

### 1. Local Bash Session
Start a local bash session, run commands, and check files:

```go
package main

import (
	"log"
	"goplur"
)

func main() {
	// Initialize local "Me" node
	node := goplur.NewMeNode()

	// Start a local bash session wrapping your operations
	err := goplur.RunBash(node, nil, func(s *goplur.Session) error {
		// Run a simple command
		output, err := s.Run("ls -la")
		if err != nil {
			return err
		}
		log.Printf("Current Directory:\n%s", output)

		// Create a backup of a file
		_, err = s.CreateBackup("/tmp/important_file.txt", ".org", false)
		return err
	})
	if err != nil {
		log.Fatalf("Session failed: %v", err)
	}
}
```

### 2. Nested SSH Session & Privilege Elevation
Push custom targets to the session stack, login via SSH/Telnet, and elevate permissions to `root` using `Sudo` or `Su`:

```go
package main

import (
	"log"
	"goplur"
)

func main() {
	// Target SSH Host Node
	sshNode := goplur.NewSshNode("webserver", "192.168.10.22", "admin", "mySecretPassword", "almalinux9")

	// Start SSH session
	err := goplur.RunSsh(sshNode, nil, func(s *goplur.Session) error {
		
		// Run commands as user 'admin'
		s.Run("whoami") // returns "admin"

		// Elevate privileges to root using Sudo wrapper
		err := goplur.Sudo(s, func(s *goplur.Session) error {
			s.Run("whoami") // returns "root"
			
			// Idempotent line append
			return s.AppendLine("export APP_ENV=production", "/etc/profile")
		})
		
		return err
	})
	if err != nil {
		log.Fatalf("SSH execution failed: %v", err)
	}
}
```

---

## Idempotent Operations

`goplur` offers multiple idempotent command helpers directly attached to the `Session` struct:

| Method | Description |
| :--- | :--- |
| `Run(cmd)` | Runs command and returns captured output. |
| `Wget(url, opts)` | Runs `wget` and returns whether download succeeded (safely handling 404s/routing errors). |
| `Patch(patchfile)` | Safely runs `patch` (gracefully declining reversed or previously applied patches). |
| `HereDoc(filePath, contents, EOF)` | Generates/writes a multi-line heredoc to a remote file path. |
| `SedReplace(srcExp, dstStr, srcFile, dstFile)` | Replaces matched regex strings inside a file using `sed`. |
| `AppendLine(line, filePath)` | Appends a line to a file only if that exact line is not already present. |
| `CheckFileExists(filePath)` | Checks if a file exists on the host. |
| `CheckCommandExists(cmd)` | Checks if a CLI command is installed and available in `$PATH`. |
| `ServiceOn(serviceName)` | Enables and starts systemd service immediately (`systemctl enable --now`). |

---

## Logging Configurations

Logging behaves according to the `LOG_PARAMS` environment variable or properties passed during initialization:

```bash
# Log only to Standard Output (default)
export LOG_PARAMS=only_stdout

# Write output logs and debug trace files in /tmp/plur_log
export LOG_PARAMS=normal

# Write logs and keep full unmatched CLI buffer sizes in debug trace
export LOG_PARAMS=debug
```

Available `LogParams` struct fields:
- `LogDir`: Directory to store output and debug trace logs.
- `EnableStdout`: Write live process output to `os.Stdout`.
- `OutputLogFilePath` / `OutputLogAppendPath`: Output logs tracking.
- `DebugLogFilePath` / `DebugLogAppendPath`: Expectation matching, actions, and timings tracing.
- `DebugColor`: Enable colored logging outputs.
- `DeleteMtime` / `DeleteMtimeUnit`: Prunes older files inside the `LogDir` on launch (e.g., delete files older than 10 days).
