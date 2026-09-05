//go:build ignore

// This example demonstrates how to handle a Telnet device that does not support
// standard shell 'exit' commands, requiring a Telnet escape sequence (Ctrl+], then quit)
// to cleanly terminate the session.
//
// A lightweight mock TCP server is started in the background to reproduce
// this exact real-world scenario (commonly found in serial consoles, embedded switches, etc.).
//
// Usage:
//   go run example/telnet.go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"

	"goplur"
)

// startMockTelnetServer starts a local TCP server that simulates a network appliance.
// Key feature: It ignores/refuses 'exit' commands and requires Telnet escape sequence to disconnect.
func startMockTelnetServer() (int, func(), error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port

	done := make(chan struct{})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					return
				}
			}
			go handleClient(conn)
		}
	}()

	cleanup := func() {
		close(done)
		_ = ln.Close()
	}

	return port, cleanup, nil
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// 1. Telnet Login & Password interaction
	conn.Write([]byte("Login: "))
	user, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	user = strings.TrimSpace(user)

	conn.Write([]byte("Password: "))
	_, err = reader.ReadString('\n')
	if err != nil {
		return
	}

	// 2. Welcome banner and command prompt
	conn.Write([]byte(fmt.Sprintf("\r\n--- Welcome %s to Embedded Appliance CLI ---\r\n", user)))
	conn.Write([]byte("appliance> "))

	// 3. Command processing loop
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Client disconnected (e.g. via Telnet escape Ctrl+] -> quit)
			return
		}
		cmd := strings.TrimSpace(line)

		switch cmd {
		case "exit", "quit":
			// Refuse exit/quit! Do NOT close the TCP connection.
			conn.Write([]byte("\r\n[!] Error: 'exit' command is disabled on this device.\r\n"))
			conn.Write([]byte("[!] To disconnect, use the Telnet escape sequence: press Ctrl+], then enter 'quit'.\r\n"))
			conn.Write([]byte("appliance> "))
		case "status":
			conn.Write([]byte("\r\n[OK] Device Status: Normal (Temperature: 38C, Power: OK)\r\n"))
			conn.Write([]byte("appliance> "))
		case "help":
			conn.Write([]byte("\r\nAvailable commands: status, help (Note: exit is disabled)\r\n"))
			conn.Write([]byte("appliance> "))
		default:
			conn.Write([]byte(fmt.Sprintf("\r\nCommand received: %s\r\n", cmd)))
			conn.Write([]byte("appliance> "))
		}
	}
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println(" Starting Telnet Example: Disconnecting via Escape Sequence (Ctrl+], then quit)")
	fmt.Println("================================================================================")

	// 1. Start mock appliance server on an ephemeral local port
	port, cleanup, err := startMockTelnetServer()
	if err != nil {
		log.Fatalf("Failed to start mock Telnet server: %v", err)
	}
	defer cleanup()

	log.Printf("Mock appliance Telnet server running on 127.0.0.1:%d", port)

	// 2. Initialize TelnetNode and configure .WithEscapeExit()
	node := goplur.NewTelnetNode("appliance", "127.0.0.1", "admin", "secretPass", "generic").
		WithEscapeExit() // <--- Automatically sends Ctrl+] and 'quit' on session exit!
	node.TelnetPort = port
	node.WaitPrompt = `appliance> `

	logParams := goplur.DefaultLogParams()

	// 3. Run Telnet session
	err = goplur.RunTelnet(node, &logParams, func(s *goplur.Session) error {
		log.Println("[1] Successfully connected and logged into appliance via Telnet!")

		// Execute a standard command
		log.Println("[2] Executing 'status' command...")
		statusOut, err := s.Run("status")
		if err != nil {
			return err
		}
		log.Printf("Status output:\n%s", statusOut)

		// Test sending 'exit' to demonstrate that server refuses it
		log.Println("[3] Testing 'exit' command (the server will refuse to disconnect)...")
		exitOut, err := s.Run("exit")
		if err != nil {
			return err
		}
		log.Printf("Exit command output:\n%s", exitOut)

		log.Println("[4] Session callback finished. RunTelnet will now trigger WithEscapeExit()...")
		return nil
	})

	if err != nil {
		log.Fatalf("Telnet session failed: %v", err)
	}

	fmt.Println("================================================================================")
	fmt.Println(" Telnet session cleanly terminated using escape sequence (Ctrl+] -> quit)!")
	fmt.Println("================================================================================")
}
