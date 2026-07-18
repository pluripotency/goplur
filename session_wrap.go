package goplur

import (
	"fmt"
)

func RunSession(node *Node, loginMethod string, logParams *LogParams, fn func(s *Session) error) error {
	s := NewSession(node, logParams)
	defer s.Close()

	var err error
	switch loginMethod {
	case "bash":
		_, err = s.Bash()
	case "ssh":
		_, err = s.Ssh()
	case "telnet":
		_, err = s.Telnet()
	default:
		return fmt.Errorf("unknown login method: %s", loginMethod)
	}
	if err != nil {
		return err
	}

	err = fn(s)
	if err != nil {
		return err
	}

	currentNode := s.CurrentNode()
	exitCmd := currentNode.ExitCommand
	if exitCmd == "" {
		exitCmd = "exit"
	}

	if len(s.nodes) == 1 {
		rows := []ExpectRow{
			{Pattern: "EOF", Reaction: ReactionSuccess, Arg: nil},
		}
		s.Do(exitCmd, rows, s.timeout)
	} else {
		s.PopNode()
		s.Run(exitCmd)
	}

	return nil
}

func RunTelnet(node *Node, logParams *LogParams, fn func(s *Session) error) error {
	return RunSession(node, "telnet", logParams, fn)
}

func RunSsh(node *Node, logParams *LogParams, fn func(s *Session) error) error {
	return RunSession(node, "ssh", logParams, fn)
}

func RunBash(node *Node, logParams *LogParams, fn func(s *Session) error) error {
	if node == nil {
		node = NewMeNode()
	}
	return RunSession(node, "bash", logParams, fn)
}

func Sudo(s *Session, fn func(s *Session) error) error {
	sudoOn := false
	if s.CurrentNode().Username != "root" {
		sudoOn = true
		_, err := s.SudoI()
		if err != nil {
			return err
		}
	}

	err := fn(s)

	if sudoOn {
		_, suErr := s.SuExit()
		if suErr != nil && err == nil {
			err = suErr
		}
	}
	return err
}

func Su(s *Session, username string, fn func(s *Session) error) error {
	if username == "" {
		username = "root"
	}
	suOn := false
	if s.CurrentNode().Username != username {
		suOn = true
		_, err := s.Su(username)
		if err != nil {
			return err
		}
	}

	err := fn(s)

	if suOn {
		_, suErr := s.SuExit()
		if suErr != nil && err == nil {
			err = suErr
		}
	}
	return err
}
