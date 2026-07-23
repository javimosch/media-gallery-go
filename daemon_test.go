package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsDaemonRunning(t *testing.T) {
	// No daemon should be running in test
	if isDaemonRunning() {
		t.Log("daemon might be running (could be real instance) — not a failure")
	}
}

func TestStopDaemonNotRunning(t *testing.T) {
	// Remove PID file to ensure daemon is "not running"
	os.Remove(pidFile)
	// stopDaemon prints "Daemon is not running" and returns
	// It shouldn't panic or exit
	stopDaemon()
}

func TestCheckDaemonStatusNotRunning(t *testing.T) {
	os.Remove(pidFile)
	// Just verify it doesn't panic
	checkDaemonStatus()
}

func TestIsDaemonRunningWithStalePID(t *testing.T) {
	// Write a stale PID file (PID of current process, but we're not a daemon)
	os.WriteFile(pidFile, []byte("99999999"), 0644)
	defer os.Remove(pidFile)

	// Should detect stale PID and return false
	if isDaemonRunning() {
		t.Error("expected false for stale PID")
	}
}

func TestAbsPath(t *testing.T) {
	result := absPath("/tmp/test")
	if result != "/tmp/test" {
		t.Errorf("expected /tmp/test, got %s", result)
	}

	// Relative path should become absolute
	result = absPath(".")
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %s", result)
	}
}
