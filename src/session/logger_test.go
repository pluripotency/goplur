package session

import (
	"strings"
	"testing"
)

func TestDefaultLogParams(t *testing.T) {
	lp := DefaultLogParams()

	if lp.LogDir != "/tmp/goplur_log" {
		t.Errorf("expected LogDir to be /tmp/goplur_log, got %s", lp.LogDir)
	}
	if !lp.EnableStdout {
		t.Errorf("expected EnableStdout to be true, got %v", lp.EnableStdout)
	}
	if !strings.HasPrefix(lp.OutputLogFilePath, "/tmp/goplur_log/") {
		t.Errorf("expected OutputLogFilePath to start with /tmp/goplur_log/, got %s", lp.OutputLogFilePath)
	}
	if !strings.HasPrefix(lp.DebugLogFilePath, "/tmp/goplur_log/") {
		t.Errorf("expected DebugLogFilePath to start with /tmp/goplur_log/, got %s", lp.DebugLogFilePath)
	}
	if !lp.DebugColor {
		t.Errorf("expected DebugColor to be true, got %v", lp.DebugColor)
	}
	if lp.DeleteMtime != 10 {
		t.Errorf("expected DeleteMtime to be 10, got %d", lp.DeleteMtime)
	}
	if lp.DeleteMtimeUnit != "day" {
		t.Errorf("expected DeleteMtimeUnit to be 'day', got %s", lp.DeleteMtimeUnit)
	}
}
