package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestPrintHelp(t *testing.T) {
	// Just verify it doesn't panic
	printHelp()
}

func TestAbsPathRelative(t *testing.T) {
	result := absPath("relative/path")
	if result == "relative/path" {
		t.Error("expected absolute path for relative input")
	}
}

func TestMainVersion(t *testing.T) {
	if os.Getenv("TEST_MAIN_VERSION") == "1" {
		os.Args = []string{"media-gallery-go", "version"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainVersion")
	cmd.Env = append(os.Environ(), "TEST_MAIN_VERSION=1")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("main version failed: %v", err)
	}
}

func TestMainHelp(t *testing.T) {
	if os.Getenv("TEST_MAIN_HELP") == "1" {
		os.Args = []string{"media-gallery-go", "help"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelp")
	cmd.Env = append(os.Environ(), "TEST_MAIN_HELP=1")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("main help failed: %v", err)
	}
}

func TestMainStartMissingMedia(t *testing.T) {
	if os.Getenv("TEST_MAIN_START_FAIL") == "1" {
		os.Args = []string{"media-gallery-go", "start", "-media=/nonexistent/path/xyz", "-db=/nonexistent/db.db"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainStartMissingMedia")
	cmd.Env = append(os.Environ(), "TEST_MAIN_START_FAIL=1")
	err := cmd.Run()
	if err == nil {
		t.Error("expected exit code 90 for missing media root")
	}
}

func TestMainStop(t *testing.T) {
	if os.Getenv("TEST_MAIN_STOP") == "1" {
		os.Remove(pidFile)
		os.Args = []string{"media-gallery-go", "stop"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainStop")
	cmd.Env = append(os.Environ(), "TEST_MAIN_STOP=1")
	err := cmd.Run()
	// stop on non-running daemon exits 0
	if err != nil {
		t.Fatalf("main stop failed: %v", err)
	}
}

func TestMainStatus(t *testing.T) {
	if os.Getenv("TEST_MAIN_STATUS") == "1" {
		os.Remove(pidFile)
		os.Args = []string{"media-gallery-go", "status"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainStatus")
	cmd.Env = append(os.Environ(), "TEST_MAIN_STATUS=1")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("main status failed: %v", err)
	}
}

func TestMainUnknownCommand(t *testing.T) {
	if os.Getenv("TEST_MAIN_UNKNOWN") == "1" {
		os.Args = []string{"media-gallery-go", "badcommand"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainUnknown")
	cmd.Env = append(os.Environ(), "TEST_MAIN_UNKNOWN=1")
	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit for unknown command")
	}
}

func TestMainNoArgs(t *testing.T) {
	if os.Getenv("TEST_MAIN_NOARGS") == "1" {
		os.Args = []string{"media-gallery-go"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainNoArgs")
	cmd.Env = append(os.Environ(), "TEST_MAIN_NOARGS=1")
	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit for no args")
	}
}

