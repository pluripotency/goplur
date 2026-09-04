//go:build ignore

// This example demonstrates how to establish an SSH session using a local system user.
//
// Usage:
// Run this program, and enter the target hostname and access IP when prompted.
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"goplur"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Starting SSH example. Enter the target hostname and access IP address")
	fmt.Println("when prompted (defaults to localhost / 127.0.0.1).")
	fmt.Println("--------------------------------------------------------------------------------")

	fmt.Print("Enter target Hostname (e.g., myhost) [default: localhost]: ")
	hostInput, _ := reader.ReadString('\n')
	hostname := strings.TrimSpace(hostInput)
	if hostname == "" {
		hostname = "localhost"
	}

	fmt.Print("Enter target Access IP (e.g., 192.168.1.100) [default: 127.0.0.1]: ")
	ipInput, _ := reader.ReadString('\n')
	accessIp := strings.TrimSpace(ipInput)
	if accessIp == "" {
		accessIp = "127.0.0.1"
	}

	username := os.Getenv("USER")
	password := ""
	port := 22
	platform := "ubuntu"

	log.Printf("Initializing SSH node for %s@%s:%d (Platform: %s)...", username, hostname, port, platform)

	// Create SSH node configuration
	node := goplur.NewSshNode(hostname, accessIp, username, password, platform)
	node.SSHPort = port

	logParams := goplur.DefaultLogParams()
	// Start SSH session wrapper
	err := goplur.RunSsh(node, &logParams, func(s *goplur.Session) error {
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

		return nil
	})

	if err != nil {
		log.Fatalf("SSH session failed: %v", err)
	}
	log.Println("SSH example run finished successfully!")
}
