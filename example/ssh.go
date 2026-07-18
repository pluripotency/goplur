//go:build ignore

// This example demonstrates how to establish an SSH session using a local system user
// and passwordless SSH key authentication.
//
// Prerequisites:
// 1. Configure passwordless SSH access to the target host beforehand using:
//    ssh-copy-id <user>@<access_ip>
// 2. Run this program, and enter the target hostname and access IP when prompted.
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
	fmt.Println("Before running this example, please configure passwordless SSH access to the")
	fmt.Println("target host using 'ssh-copy-id'. Once configured, simply provide the target")
	fmt.Println("hostname and access IP address when prompted.")
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

	// Start SSH session wrapper
	err := goplur.RunSsh(node, nil, func(s *goplur.Session) error {
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
