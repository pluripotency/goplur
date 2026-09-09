package session

import (
	"fmt"
	nd "goplur/src/node"
)

func RunSession(n nd.Node, loginMethod string, logParams *LogParams, fn func(s *Session) error) error {
	s := NewSession(n, logParams)
	defer s.Close()

	var err error
	if handler := n.GetConnectHandler(); handler != nil {
		err = handler(s, n)
	} else {
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
	}
	if err != nil {
		return err
	}

	err = fn(s)
	if err != nil {
		return err
	}

	currentNode := s.CurrentNode()
	if exitHandler := currentNode.GetExitHandler(); exitHandler != nil {
		if len(s.nodes) > 1 {
			s.PopNode()
		}
		if err := exitHandler(s, currentNode); err != nil {
			return err
		}
	} else {
		exitCmd := currentNode.GetExitCommand()
		if exitCmd != "" {
			if len(s.nodes) == 1 {
				s.actionHandler(exitCmd)
			} else {
				s.PopNode()
				s.Run(exitCmd)
			}
		}
	}

	return nil
}

func enableDirectMode(n nd.Node) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *nd.SshNode:
		v.DirectMode = true
	case *nd.TelnetNode:
		v.DirectMode = true
	case *nd.BaseNode:
		v.DirectMode = true
	}
}

func RunDirectSsh(n nd.Node, logParams *LogParams, fn func(s *Session) error) error {
	enableDirectMode(n)
	return RunSsh(n, logParams, fn)
}

func RunDirectTelnet(n nd.Node, logParams *LogParams, fn func(s *Session) error) error {
	enableDirectMode(n)
	return RunTelnet(n, logParams, fn)
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
