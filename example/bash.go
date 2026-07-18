//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"goplur"
)

func main() {
	log.Println("Initializing Me Node...")
	// Create a node representing the current local machine/user
	node := goplur.NewMeNode()

	log.Printf("Starting Bash Session for Host: %s, User: %s, Platform: %s...", node.Hostname, node.Username, node.Platform)

	testFile := "/tmp/goplur_example_bash.txt"
	defer os.Remove(testFile)

	// Run bash session
	err := goplur.RunBash(node, nil, func(s *goplur.Session) error {
		// 1. Run a command
		log.Println("Running 'uptime'...")
		uptime, err := s.Run("uptime")
		if err != nil {
			return err
		}
		fmt.Printf("System Uptime: %s\n", stringsTrim(uptime))

		// 2. Perform HereDoc writing
		log.Printf("Writing multi-line HereDoc to %s...", testFile)
		content := "Goplur Bash Example Line 1\nTarget Config Line 2\nDone!"
		err = s.HereDoc(testFile, content, "'EOF'")
		if err != nil {
			return err
		}

		// 3. Check if file exists
		exists, err := s.CheckFileExists(testFile)
		if err != nil {
			return err
		}
		log.Printf("File exists check: %t", exists)

		// 4. Output the written file
		log.Printf("Reading back file %s:", testFile)
		catOut, err := s.Run("cat " + testFile)
		if err != nil {
			return err
		}
		fmt.Println(catOut)

		return nil
	})

	if err != nil {
		log.Fatalf("Bash example failed: %v", err)
	}
	log.Println("Bash example completed successfully!")
}

// Simple helper to trim output spaces/newlines
func stringsTrim(s string) string {
	return strings.TrimSpace(s)
}
