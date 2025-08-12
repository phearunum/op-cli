// manager/process.go
package manager

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

type ManagedProcess struct {
	Config     *AppConfig
	cmd        *exec.Cmd
	stdoutPipe io.ReadCloser
	stderrPipe io.ReadCloser
	startTime  time.Time
	mu         sync.Mutex
	PingStatus string
	Message    string
}

type ProcessMetrics struct {
	CPU    float64
	Memory uint64
}

func NewManagedProcess(cfg *AppConfig) *ManagedProcess {
	return &ManagedProcess{
		Config:     cfg,
		mu:         sync.Mutex{},
		Message:    "Starting...",
		PingStatus: "N/A",
	}
}

func (mp *ManagedProcess) Start() {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	log.Printf("Starting process '%s' with command: %s\n", mp.Config.Name, mp.Config.Command)

	parts := strings.Fields(mp.Config.Command)
	if len(parts) == 0 {
		mp.Message = "Invalid command"
		return
	}

	name := parts[0]
	args := parts[1:]

	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var err error
	if mp.stdoutPipe, err = cmd.StdoutPipe(); err != nil {
		log.Printf("Failed to create stdout pipe for '%s': %v", mp.Config.Name, err)
		mp.Message = "Stdout pipe error"
		return
	}
	if mp.stderrPipe, err = cmd.StderrPipe(); err != nil {
		log.Printf("Failed to create stderr pipe for '%s': %v", mp.Config.Name, err)
		mp.Message = "Stderr pipe error"
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start process '%s': %v", mp.Config.Name, err)
		mp.Message = fmt.Sprintf("Failed to start: %v", err)
		return
	}

	mp.cmd = cmd
	mp.startTime = time.Now()
	mp.Message = "Running"

	go mp.readStdout()
	go mp.readStderr()

	go func() {
		_ = cmd.Wait()
		log.Printf("Process '%s' exited.", mp.Config.Name)
		mp.mu.Lock()
		mp.Message = "Exited"
		mp.mu.Unlock()
	}()
}

func (mp *ManagedProcess) Stop() {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.cmd != nil && mp.cmd.Process != nil {
		pgid, err := syscall.Getpgid(mp.cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			mp.cmd.Process.Kill()
		}
		mp.cmd = nil
	}
}

func (mp *ManagedProcess) getProcessMetrics() (*ProcessMetrics, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.cmd == nil || mp.cmd.Process == nil {
		return nil, fmt.Errorf("process not running")
	}

	pid := int32(mp.cmd.Process.Pid)
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	cpuPercent, err := p.CPUPercent()
	if err != nil {
		return nil, err
	}

	memInfo, err := p.MemoryInfo()
	if err != nil {
		return nil, err
	}

	return &ProcessMetrics{
		CPU:    cpuPercent,
		Memory: memInfo.RSS,
	}, nil
}

func (mp *ManagedProcess) readStdout() {
	defer mp.stdoutPipe.Close()
	scanner := bufio.NewScanner(mp.stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()
		mp.mu.Lock()
		if strings.HasPrefix(mp.Config.Name, "ping-") {
			if strings.Contains(line, "Request timeout") || strings.Contains(line, "Destination Host Unreachable") {
				mp.PingStatus = "TIMEOUT"
			} else if strings.Contains(line, "bytes from") {
				mp.PingStatus = "OK"
			}
		}
		mp.mu.Unlock()
		log.Printf("stdout (%s): %s", mp.Config.Name, line)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading stdout for '%s': %v", mp.Config.Name, err)
	}
}

func (mp *ManagedProcess) readStderr() {
	defer mp.stderrPipe.Close()
	scanner := bufio.NewScanner(mp.stderrPipe)
	for scanner.Scan() {
		log.Printf("stderr (%s): %s", mp.Config.Name, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading stderr for '%s': %v", mp.Config.Name, err)
	}
}

func (mp *ManagedProcess) updatePingStatus() {
	// This function is now empty as the ping status is updated in real-time in readStdout.
}
