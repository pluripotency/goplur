//go:build ignore

package main

import (
	"flag"
	"log"
	"os"
	"strings"

	"goplur"
)

func main() {
	// Parse SSH parameters from flags or fallback to defaults
	hostname := flag.String("host", "localhost", "Target SSH host")
	accessIp := flag.String("ip", "127.0.0.1", "Target SSH ip")
	username := flag.String("user", os.Getenv("USER"), "SSH username (defaults to current system user)")
	password := flag.String("pass", "", "SSH password")
	port := flag.Int("port", 22, "SSH port")
	platform := flag.String("platform", "ubuntu", "Target machine platform (e.g. ubuntu, almalinux9)")
	flag.Parse()

	log.Printf("Initializing SSH node for %s@%s:%d (Platform: %s)...", *username, *hostname, *port, *platform)

	// Create SSH node configuration
	node := goplur.NewSshNode(*hostname, *accessIp, *username, *password, *platform)
	node.SSHPort = *port

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
