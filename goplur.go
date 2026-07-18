package goplur

import (
	"goplur/src/expect"
	"goplur/src/node"
	"goplur/src/session"
)

// Re-export Node types from src/node
type Node = node.Node
type BaseNode = node.BaseNode
type BashNode = node.BashNode
type SshNode = node.SshNode
type TelnetNode = node.TelnetNode

// Re-export Session and related types from src/session
type Session = session.Session
type ExpectRow = session.ExpectRow
type LogParams = session.LogParams
type ReactionType = session.ReactionType
type SessionLogger = session.SessionLogger
type DebugLogger = session.DebugLogger

// Re-export expect types from src/expect
type GExpect = expect.GExpect
type Caser = expect.Caser
type Case = expect.Case
type Tag = expect.Tag

// Re-export constants
const (
	ReactionSuccess     = session.ReactionSuccess
	ReactionSend        = session.ReactionSend
	ReactionSendLine    = session.ReactionSendLine
	ReactionSendPass    = session.ReactionSendPass
	ReactionGetPass     = session.ReactionGetPass
	ReactionSendControl = session.ReactionSendControl
	ReactionExit        = session.ReactionExit
)

const (
	OKTag = expect.OKTag
	NoTag = expect.NoTag
)

const DefaultTimeout = expect.DefaultTimeout

// Re-export functions
func NewMeNode() *node.BashNode {
	return node.NewMeNode()
}

func NewSshNode(hostname, accessIp, username, password, platform string) *node.SshNode {
	return node.NewSshNode(hostname, accessIp, username, password, platform)
}

func NewTelnetNode(hostname, accessIp, username, password, platform string) *node.TelnetNode {
	return node.NewTelnetNode(hostname, accessIp, username, password, platform)
}

func NewSession(rootNode Node, logParams *LogParams) *Session {
	return session.NewSession(rootNode, logParams)
}

func RunBash(n Node, logParams *LogParams, fn func(s *Session) error) error {
	return session.RunBash(n, logParams, fn)
}

func RunSsh(n Node, logParams *LogParams, fn func(s *Session) error) error {
	return session.RunSsh(n, logParams, fn)
}

func RunTelnet(n Node, logParams *LogParams, fn func(s *Session) error) error {
	return session.RunTelnet(n, logParams, fn)
}

func Sudo(s *Session, fn func(s *Session) error) error {
	return session.Sudo(s, fn)
}

func Su(s *Session, username string, fn func(s *Session) error) error {
	return session.Su(s, username, fn)
}

func SelectLogParams(envStr string) LogParams {
	return session.SelectLogParams(envStr)
}

func OK() func() (Tag, error) {
	return expect.OK()
}
