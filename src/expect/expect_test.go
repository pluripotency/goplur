package expect

import (
	"regexp"
	"testing"
	"time"
)

func TestLocalSpawnEcho(t *testing.T) {
	exp, _, err := Spawn("echo hello_goexpect", 5*time.Second)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer exp.Close()

	re := regexp.MustCompile("hello_goexpect")
	out, match, err := exp.Expect(re, 2*time.Second)
	if err != nil {
		t.Fatalf("Expect failed: %v", err)
	}
	if len(match) == 0 {
		t.Errorf("Expected match, got none. Output was: %q", out)
	}
}

func TestLocalExpectTimeout(t *testing.T) {
	exp, _, err := Spawn("sleep 2", 5*time.Second)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer exp.Close()

	re := regexp.MustCompile("never_matched")
	_, _, err = exp.Expect(re, 50*time.Millisecond)
	if err == nil {
		t.Fatal("Expected TimeoutError, got nil")
	}

	if _, ok := err.(TimeoutError); !ok {
		t.Errorf("Expected TimeoutError type, got: %T (%v)", err, err)
	}
}

func TestLocalExpectSwitchCase(t *testing.T) {
	exp, _, err := Spawn("echo response_pattern", 5*time.Second)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer exp.Close()

	cases := []Caser{
		&Case{
			R: regexp.MustCompile("wrong_pattern"),
			T: OK(),
		},
		&Case{
			R: regexp.MustCompile("response_pattern"),
			T: OK(),
		},
	}

	out, match, idx, err := exp.ExpectSwitchCase(cases, 2*time.Second)
	if err != nil {
		t.Fatalf("ExpectSwitchCase failed: %v", err)
	}
	if idx != 1 {
		t.Errorf("Expected matching case index 1, got %d. Output: %q, matches: %v", idx, out, match)
	}
}
