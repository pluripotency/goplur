package session

import (
	"fmt"
	nd "goplur/src/node"
)

func RunSession(n nd.Node, loginMethod string, logParams *LogParams, fn func(s *Session) error) error {
	s := NewSession(n, logParams)
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
	exitCmd := currentNode.GetExitCommand()

	if len(s.nodes) == 1 {
		s.actionHandler(exitCmd)
	} else {
		s.PopNode()
		s.Run(exitCmd)
	}

	return nil
}

func RunTelnet(n nd.Node, logParams *LogParams, fn func(s *Session) error) error {
	return RunSession(n, "telnet", logParams, fn)
}

func RunSsh(n nd.Node, logParams *LogParams, fn func(s *Session) error) error {
	return RunSession(n, "ssh", logParams, fn)
}

func RunBash(n nd.Node, logParams *LogParams, fn func(s *Session) error) error {
	if n == nil {
		n = nd.NewMeNode()
	}
	return RunSession(n, "bash", logParams, fn)
}

func Sudo(s *Session, fn func(s *Session) error) error {
	sudoOn := false
	if s.CurrentNode().GetUsername() != "root" {
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
	if s.CurrentNode().GetUsername() != username {
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
