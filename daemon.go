package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const (
	pidFile = "/tmp/media-gallery-go.pid"
	logFile = "/tmp/media-gallery-go.log"
)

func startDaemon(cfg ServerConfig) {
	if isDaemonRunning() {
		fmt.Println("Daemon is already running")
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}

	args := []string{"start",
		fmt.Sprintf("-port=%d", cfg.Port),
		fmt.Sprintf("-media=%s", cfg.MediaRoot),
		fmt.Sprintf("-db=%s", cfg.DBPath),
		fmt.Sprintf("-thumbs=%s", cfg.ThumbDir),
		fmt.Sprintf("-max-dl=%d", cfg.MaxConcurrent),
	}
	if cfg.AuthUser != "" {
		args = append(args, fmt.Sprintf("-user=%s", cfg.AuthUser))
	}
	if cfg.AuthPass != "" {
		args = append(args, fmt.Sprintf("-pass=%s", cfg.AuthPass))
	}

	cmd := exec.Command(execPath, args...)

	logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer logFileHandle.Close()

	cmd.Stdout = logFileHandle
	cmd.Stderr = logFileHandle
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
		os.Exit(1)
	}

	pid := cmd.Process.Pid
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)

	fmt.Printf("Daemon started with PID %d\n", pid)
	fmt.Printf("Logs: %s\n", logFile)
}

func stopDaemon() {
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Daemon is not running")
			return
		}
		fmt.Fprintf(os.Stderr, "Error reading PID file: %v\n", err)
		os.Exit(1)
	}

	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding process: %v\n", err)
		os.Exit(1)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping process: %v\n", err)
		os.Exit(1)
	}

	os.Remove(pidFile)
	fmt.Printf("Daemon stopped (PID %d)\n", pid)
}

func checkDaemonStatus() {
	if isDaemonRunning() {
		pidData, _ := os.ReadFile(pidFile)
		fmt.Printf("Daemon is running (PID %s)\n", strings.TrimSpace(string(pidData)))
		fmt.Printf("Logs: %s\n", logFile)
	} else {
		fmt.Println("Daemon is not running")
	}
}

func isDaemonRunning() bool {
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return false
	}
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		os.Remove(pidFile)
		return false
	}
	return true
}
