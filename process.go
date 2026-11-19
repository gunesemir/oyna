//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type launchExit struct{ code int }

func (e *launchExit) Error() string { return fmt.Sprintf("game exited with status %d", e.code) }

func processTree(root int) map[int]bool {
	parents := map[int]int{}
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "status"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PPid:") {
				parents[pid], _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
				break
			}
		}
	}
	result := map[int]bool{root: true}
	for changed := true; changed; {
		changed = false
		for pid, parent := range parents {
			if result[parent] && !result[pid] {
				result[pid] = true
				changed = true
			}
		}
	}
	return result
}
func reapOrphans(exclude int) {
	// Wait only for known zombie children, preserving exec.Cmd's ownership of
	// its direct child and avoiding waits on unrelated goroutines' subprocesses.
	for pid := range processTree(os.Getpid()) {
		if pid == os.Getpid() || pid == exclude {
			continue
		}
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "State:\tZ") {
			var status syscall.WaitStatus
			_, _ = syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
		}
	}
}
func supervise(command []string, cwd string) error {
	if len(command) == 0 {
		return fmt.Errorf("empty launch command")
	}
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("enable child subreaper: %w", err)
	}
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitorGamescope(ctx)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var childErr error
	childDone := false
	for {
		select {
		case sig := <-signals:
			// Some descendants start new sessions, so also signal the whole tree.
			for pid := range processTree(os.Getpid()) {
				if pid != os.Getpid() {
					_ = syscall.Kill(pid, sig.(syscall.Signal))
				}
			}
		case childErr = <-done:
			childDone = true
			done = nil
		case <-ticker.C:
		}
		exclude := cmd.Process.Pid
		if childDone {
			exclude = 0
		}
		reapOrphans(exclude)
		if childDone && len(processTree(os.Getpid())) == 1 {
			break
		}
	}
	if childErr == nil {
		return nil
	}
	var exit *exec.ExitError
	if errors.As(childErr, &exit) {
		code := exit.ExitCode()
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			code = 128 + int(status.Signal())
		}
		return &launchExit{code}
	}
	return childErr
}
