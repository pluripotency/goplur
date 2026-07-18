package goplur

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogParams struct {
	LogDir              string `json:"log_dir"`
	EnableStdout        bool   `json:"enable_stdout"`
	OutputLogFilePath   string `json:"output_log_file_path"`
	OutputLogAppendPath string `json:"output_log_append_path"`
	DebugLogFilePath    string `json:"debug_log_file_path"`
	DebugLogAppendPath  string `json:"debug_log_append_path"`
	DontTruncate        bool   `json:"dont_truncate"`
	DebugColor          bool   `json:"debug_color"`
	DeleteMtime         int    `json:"delete_mtime"`
	DeleteMtimeUnit     string `json:"delete_mtime_unit"` // "sec", "min", "hour", "day"
}

// ForkWriter writes to multiple files and syncs/closes them safely
type ForkWriter struct {
	mu    sync.Mutex
	files []*os.File
}

func NewForkWriter(path string, appendMode bool) (*ForkWriter, error) {
	if path == "" {
		return &ForkWriter{}, nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return nil, err
	}
	return &ForkWriter{files: []*os.File{f}}, nil
}

func (fw *ForkWriter) Write(p []byte) (n int, err error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	for _, f := range fw.files {
		f.Write(p)
	}
	return len(p), nil
}

func (fw *ForkWriter) Flush() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	for _, f := range fw.files {
		f.Sync()
	}
}

func (fw *ForkWriter) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	var lastErr error
	for _, f := range fw.files {
		f.Sync()
		if err := f.Close(); err != nil {
			lastErr = err
		}
	}
	fw.files = nil
	return lastErr
}

// SessionWriter wraps standard stdout and file output writers
type SessionWriter struct {
	mu           sync.Mutex
	writers      []io.Writer
	closers      []io.Closer
	enableStdout bool
	muted        bool
}

func (sw *SessionWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.muted {
		return len(p), nil
	}
	if sw.enableStdout {
		os.Stdout.Write(p)
	}
	for _, w := range sw.writers {
		w.Write(p)
	}
	return len(p), nil
}

func (sw *SessionWriter) Mute() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.muted = true
}

func (sw *SessionWriter) Unmute() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.muted = false
}

func (sw *SessionWriter) Close() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	for _, c := range sw.closers {
		c.Close()
	}
	sw.writers = nil
	sw.closers = nil
}

type DebugLogger struct {
	filePath     string
	appendPath   string
	f            *ForkWriter
	af           *ForkWriter
	dontTruncate bool
	debugColor   bool
}

const (
	ColorReset     = "\033[0m"
	ColorRed       = "\033[31m"
	ColorGreen     = "\033[32m"
	ColorYellow    = "\033[33m"
	ColorBlue      = "\033[34m"
	ColorPurple    = "\033[35m"
	ColorCyan      = "\033[36m"
	ColorWhite     = "\033[37m"
	ColorBrown     = "\033[33m" // Or dark yellow
	ColorPink      = "\033[35m"
	ColorLightCyan = "\033[96m"
)

func NewDebugLogger(params LogParams) *DebugLogger {
	dl := &DebugLogger{
		filePath:     params.DebugLogFilePath,
		appendPath:   params.DebugLogAppendPath,
		dontTruncate: params.DontTruncate,
		debugColor:   params.DebugColor,
	}
	if dl.filePath != "" {
		f, _ := NewForkWriter(dl.filePath, false)
		dl.f = f
	}
	if dl.appendPath != "" {
		af, _ := NewForkWriter(dl.appendPath, true)
		dl.af = af
	}
	return dl
}

func (dl *DebugLogger) colorize(str string, color string) string {
	if dl.debugColor {
		return color + str + ColorReset
	}
	return str
}

func (dl *DebugLogger) nowDebug() string {
	return time.Now().Format("2006/01/02 15:04:05")
}

func (dl *DebugLogger) Write(p []byte) (n int, err error) {
	if dl.f != nil {
		dl.f.Write(p)
	}
	if dl.af != nil {
		dl.af.Write(p)
	}
	return len(p), nil
}

func (dl *DebugLogger) writeLine(msg string) {
	dl.Write([]byte(msg + "\n"))
}

type NoCloseWriter struct {
	W io.Writer
}

func (ncw NoCloseWriter) Write(p []byte) (n int, err error) {
	return ncw.W.Write(p)
}

func (ncw NoCloseWriter) Close() error {
	return nil
}

func (dl *DebugLogger) Message(msg string) {
	dl.writeLine(fmt.Sprintf("%s %s", dl.nowDebug(), msg))
	dl.flush()
}

func (dl *DebugLogger) onAction(s *Session, action string) {
	if dl.filePath == "" && dl.appendPath == "" {
		return
	}
	dl.writeLine(fmt.Sprintf("%s action sent: %s", dl.nowDebug(), dl.colorize(action, ColorYellow)))
	node := s.CurrentNode()
	if node != nil {
		attrs := []struct {
			name string
			val  string
		}{
			{"hostname", node.GetHostname()},
			{"access_ip", node.GetAccessIP()},
			{"username", node.GetUsername()},
			{"waitprompt", node.GetWaitPrompt()},
			{"platform", node.GetPlatform()},
		}
		for _, attr := range attrs {
			dl.writeLine(dl.colorize(fmt.Sprintf("         %8s: %s", attr.name, attr.val), ColorLightCyan))
		}
	}
	dl.writeLine(dl.colorize(fmt.Sprintf("         %8s: %v", "timeout", s.timeout), ColorLightCyan))
	dl.flush()
}

func (dl *DebugLogger) beforeSelect(s *Session, rows []ExpectRow, patterns []string) {
	if dl.filePath == "" && dl.appendPath == "" {
		return
	}
	dl.writeLine(fmt.Sprintf("%s selection:", dl.nowDebug()))
	for i, row := range rows {
		dl.writeLine(dl.colorize(fmt.Sprintf("%d       expect: %q", i, patterns[i]), ColorPink))
		dl.writeLine(dl.colorize(fmt.Sprintf("      reaction: %s", reactionName(row.Reaction)), ColorPurple))
		if row.Arg != nil {
			dl.writeLine(dl.colorize(fmt.Sprintf("          args: %v", row.Arg), ColorPurple))
		}
	}
	dl.flush()
}

func (dl *DebugLogger) afterSelect(idx int, out string) {
	if dl.filePath == "" && dl.appendPath == "" {
		return
	}
	dl.writeLine(fmt.Sprintf("%s%s", dl.nowDebug(), dl.colorize(fmt.Sprintf(" selected: %d", idx), ColorPink)))
	displayOut := out
	if !dl.dontTruncate && len(out) > 160 {
		displayOut = "...truncated: " + out[len(out)-160:]
	}
	dl.writeLine(dl.colorize(fmt.Sprintf("        before: %q", displayOut), ColorBrown))
	dl.flush()
}

func (dl *DebugLogger) atRowMethod(msg string) {
	dl.writeLine(fmt.Sprintf("%s %s", dl.nowDebug(), dl.colorize(msg, ColorBrown)))
	dl.flush()
}

func (dl *DebugLogger) flush() {
	if dl.f != nil {
		dl.f.Flush()
	}
	if dl.af != nil {
		dl.af.Flush()
	}
}

func (dl *DebugLogger) Close() {
	if dl.f != nil {
		dl.f.Close()
	}
	if dl.af != nil {
		dl.af.Close()
	}
}

func reactionName(r ReactionType) string {
	switch r {
	case ReactionSuccess:
		return "success"
	case ReactionSend:
		return "send"
	case ReactionSendLine:
		return "send_line"
	case ReactionSendPass:
		return "send_pass"
	case ReactionGetPass:
		return "get_pass"
	case ReactionSendControl:
		return "send_control"
	case ReactionExit:
		return "exit"
	case ReactionCapture:
		return "p_capture"
	default:
		return "unknown"
	}
}

type SessionLogger struct {
	outputWriter *SessionWriter
	debugLog     *DebugLogger
}

func runDeleteMtime(params LogParams) {
	if params.LogDir == "" || params.DeleteMtime <= 0 {
		return
	}
	var duration time.Duration
	switch params.DeleteMtimeUnit {
	case "day":
		duration = time.Duration(params.DeleteMtime) * 24 * time.Hour
	case "hour":
		duration = time.Duration(params.DeleteMtime) * time.Hour
	case "min":
		duration = time.Duration(params.DeleteMtime) * time.Minute
	default: // "sec"
		duration = time.Duration(params.DeleteMtime) * time.Second
	}
	cutoff := time.Now().Add(-duration)

	filepath.Walk(params.LogDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == params.LogDir {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			os.RemoveAll(path)
		}
		return nil
	})
}

func NewSessionLogger(params LogParams) *SessionLogger {
	runDeleteMtime(params)

	sw := &SessionWriter{
		enableStdout: params.EnableStdout,
	}

	if params.OutputLogFilePath != "" {
		if f, err := NewForkWriter(params.OutputLogFilePath, false); err == nil {
			sw.writers = append(sw.writers, f)
			sw.closers = append(sw.closers, f)
		}
	}
	if params.OutputLogAppendPath != "" {
		if af, err := NewForkWriter(params.OutputLogAppendPath, true); err == nil {
			sw.writers = append(sw.writers, af)
			sw.closers = append(sw.closers, af)
		}
	}

	return &SessionLogger{
		outputWriter: sw,
		debugLog:     NewDebugLogger(params),
	}
}

func (sl *SessionLogger) Close() {
	sl.outputWriter.Close()
	sl.debugLog.Close()
}
