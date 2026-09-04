//go:build ignore

// This example demonstrates how to establish an SSH session using a local system user.
//
// Usage:
// Run this program, and enter the target hostname and access IP when prompted.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"goplur"
	"goplur/src/tool"
)

func main() {
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Starting SSH example. Configure SSH target node parameters interactively.")
	fmt.Println("--------------------------------------------------------------------------------")

	// JSON default values for node parameters (omitted keys inherit interactive defaults)
	jsonDefaults := []byte(`{
		"hostname": "localhost",
		"access_ip": "127.0.0.1",
		"platform": "ubuntu"
	}`)

	// 1. Validate JSON defaults and merge with interactive defaults for missing keys
	defaults, err := tool.LoadDefaultsFromJSON(jsonDefaults)
	if err != nil {
		log.Fatalf("Failed to initialize defaults from JSON: %v", err)
	}

	// 2. Interactively configure node using validated defaults
	node, err := tool.PromptSshNodeWithDefaults(os.Stdin, os.Stdout, defaults)
	if err != nil {
		log.Fatalf("Failed to configure SSH node: %v", err)
	}

	log.Printf("Initializing SSH node for %s@%s:%d (Platform: %s)...", node.Username, node.Hostname, node.SSHPort, node.Platform)

	logParams := goplur.DefaultLogParams()
	// Start SSH session wrapper
	err = goplur.RunSsh(node, &logParams, func(s *goplur.Session) error {
		log.Println("Successfully logged in via SSH!")

		// Run standard command
		log.Println("Executing 'uname -a'...")
		unameOut, err := s.Run("uname -a")
		if err != nil {
			return err
		}
		log.Printf("Remote system kernel: %s", strings.TrimSpace(unameOut))

		// Check if docker command exists on the remote system
		log.Println("Checking if docker exists...")
		dockerOk, err := s.CheckCommandExists("docker")
		if err != nil {
			return err
		}
		log.Printf("Docker command exists: %t", dockerOk)

		// Hand over control to interactive user session (pexpect.interact style)
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Println("Starting interactive terminal session (pexpect.interact style).")
		fmt.Println("You have direct shell access. Type 'exit' or press Ctrl+D to disconnect.")
		fmt.Println("--------------------------------------------------------------------------------")
		return s.Interact()
	})

	if err != nil {
		log.Fatalf("SSH session failed: %v", err)
	}
	log.Println("SSH example run finished successfully!")
}
