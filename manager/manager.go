package manager

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	socketPath = "/tmp/op_daemon.sock"
	ListFile   = "pm_processes.json"

	CmdStatus     = "status"
	CmdStop       = "stop"
	CmdCreate     = "create"
	CmdCreatePing = "create_ping"
	CmdPingStatus = "ping_status"
)

type AppConfig struct {
	Name    string
	Command string
	Port    int
}

type ProcessManager struct {
	mu           sync.Mutex
	processes    map[string]*ManagedProcess
	listener     net.Listener
	shutdownChan chan struct{}
}

type IPCMessage struct {
	Command string
	Payload string
}

type ProcessInfo struct {
	Name       string
	PID        int32
	Status     string
	Message    string
	Uptime     string
	CPU        float64
	Memory     uint64
	Port       int
	PingStatus string
}

func NewManager() *ProcessManager {
	return &ProcessManager{
		processes:    make(map[string]*ManagedProcess),
		shutdownChan: make(chan struct{}),
	}
}

func (pm *ProcessManager) AddProcess(cfg *AppConfig) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc := NewManagedProcess(cfg)
	pm.processes[cfg.Name] = proc
}

func (pm *ProcessManager) StartAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, proc := range pm.processes {
		proc.Start()
	}
}

func (pm *ProcessManager) RunAsDaemon() {
	if err := os.RemoveAll(socketPath); err != nil {
		log.Printf("Failed to remove old socket: %v", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}
	pm.listener = listener

	log.Println("Daemon is ready to accept connections...")

	go pm.acceptConnections()
	go pm.monitorProcesses()

	<-pm.shutdownChan
}

func (pm *ProcessManager) Shutdown() {
	log.Println("Shutting down processes...")
	pm.mu.Lock()
	for _, proc := range pm.processes {
		proc.Stop()
	}
	pm.mu.Unlock()

	log.Println("Closing socket listener...")
	if pm.listener != nil {
		pm.listener.Close()
	}

	close(pm.shutdownChan)
	os.Remove(socketPath)
	log.Println("Daemon shutdown complete.")
}

func (pm *ProcessManager) acceptConnections() {
	for {
		conn, err := pm.listener.Accept()
		if err != nil {
			select {
			case <-pm.shutdownChan:
				return
			default:
				log.Printf("Failed to accept connection: %v", err)
				continue
			}
		}
		go pm.handleConnection(conn)
	}
}

func (pm *ProcessManager) handleConnection(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	var msg IPCMessage
	if err := decoder.Decode(&msg); err != nil {
		log.Printf("Failed to decode message: %v", err)
		return
	}

	// Lock for the entire connection handling to prevent race conditions
	pm.mu.Lock()
	defer pm.mu.Unlock()

	switch msg.Command {
	case CmdStatus:
		pm.handleStatusRequest(conn, false)
	case CmdPingStatus:
		pm.handleStatusRequest(conn, true)
	case CmdCreate:
		pm.handleCreateRequest(conn, msg.Payload)
	case CmdCreatePing:
		pm.handleCreatePingRequest(conn, msg.Payload)
	case CmdStop:
		pm.handleStopRequest(conn, msg.Payload)
	}
}

func (pm *ProcessManager) handleStatusRequest(conn net.Conn, isPing bool) {
	var infos []*ProcessInfo
	for _, proc := range pm.processes {
		if isPing && !strings.HasPrefix(proc.Config.Name, "ping-") {
			continue
		}
		if !isPing && strings.HasPrefix(proc.Config.Name, "ping-") {
			continue
		}

		info := pm.getProcessInfo(proc)
		infos = append(infos, info)
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(infos); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

func (pm *ProcessManager) handleCreateRequest(conn net.Conn, payload string) {
	var cfg AppConfig
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		json.NewEncoder(conn).Encode(map[string]string{"error": fmt.Sprintf("Invalid payload: %v", err)})
		return
	}

	if _, ok := pm.processes[cfg.Name]; ok {
		json.NewEncoder(conn).Encode(map[string]string{"error": fmt.Sprintf("Service '%s' already exists.", cfg.Name)})
		return
	}

	proc := NewManagedProcess(&cfg)
	pm.processes[cfg.Name] = proc
	proc.Start()

	if err := SaveApp(proc.Config, ListFile); err != nil {
		log.Printf("Failed to save app list: %v", err)
	}

	json.NewEncoder(conn).Encode(map[string]string{"message": fmt.Sprintf("Service '%s' created and started.", cfg.Name)})
}

func (pm *ProcessManager) handleCreatePingRequest(conn net.Conn, payload string) {
	var data map[string]string
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		json.NewEncoder(conn).Encode(map[string]string{"error": fmt.Sprintf("Invalid payload: %v", err)})
		return
	}
	ipAddress := data["ip"]
	serviceName := "ping-" + ipAddress

	if _, ok := pm.processes[serviceName]; ok {
		json.NewEncoder(conn).Encode(map[string]string{"error": fmt.Sprintf("Ping service for '%s' already exists.", ipAddress)})
		return
	}

	cfg := &AppConfig{
		Name:    serviceName,
		Command: fmt.Sprintf("ping -i 1 %s", ipAddress),
	}
	proc := NewManagedProcess(cfg)
	pm.processes[serviceName] = proc
	proc.Start()

	if err := SaveApp(proc.Config, ListFile); err != nil {
		log.Printf("Failed to save app list: %v", err)
	}

	json.NewEncoder(conn).Encode(map[string]string{"message": fmt.Sprintf("Ping service for '%s' created and started.", ipAddress)})
}

func (pm *ProcessManager) handleStopRequest(conn net.Conn, serviceName string) {
	proc, ok := pm.processes[serviceName]
	if !ok {
		json.NewEncoder(conn).Encode(map[string]string{"error": fmt.Sprintf("Service '%s' not found.", serviceName)})
		return
	}

	proc.Stop()
	delete(pm.processes, serviceName)

	if err := RemoveApp(serviceName, ListFile); err != nil {
		log.Printf("Failed to remove app from list: %v", err)
	}

	json.NewEncoder(conn).Encode(map[string]string{"message": fmt.Sprintf("Service '%s' stopped and removed.", serviceName)})
}

func (pm *ProcessManager) getProcessInfo(proc *ManagedProcess) *ProcessInfo {
	info := &ProcessInfo{
		Name:       proc.Config.Name,
		PID:        0,
		Status:     "stopped",
		Uptime:     "N/A",
		CPU:        0.0,
		Memory:     0,
		Port:       proc.Config.Port,
		PingStatus: proc.PingStatus,
	}

	if proc.cmd != nil && proc.cmd.Process != nil {
		err := proc.cmd.Process.Signal(syscall.Signal(0))
		if err == nil {
			info.PID = int32(proc.cmd.Process.Pid)
			info.Status = "running"
			info.Uptime = time.Since(proc.startTime).Round(time.Second).String()

			metrics, err := proc.getProcessMetrics()
			if err == nil {
				info.CPU = metrics.CPU
				info.Memory = metrics.Memory
			}
		} else if err.Error() == "no such process" {
			info.Status = "exited"
			info.Uptime = "N/A"
		} else {
			info.Status = "unknown"
			info.Uptime = "N/A"
		}
	}

	info.Message = proc.Message
	return info
}

func (pm *ProcessManager) monitorProcesses() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pm.shutdownChan:
			return
		case <-ticker.C:
			pm.mu.Lock()
			for _, proc := range pm.processes {
				if proc.cmd != nil && proc.cmd.Process != nil {
					err := proc.cmd.Process.Signal(syscall.Signal(0))
					if err != nil && err.Error() == "no such process" {
						log.Printf("Process %s (PID: %d) has exited. Restarting...", proc.Config.Name, proc.cmd.Process.Pid)
						proc.Stop()
						proc.Start()
					}
				}
				if strings.HasPrefix(proc.Config.Name, "ping-") {
					proc.updatePingStatus()
				}
			}
			pm.mu.Unlock()
		}
	}
}

func SaveApp(app *AppConfig, filePath string) error {
	apps, err := LoadApps(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		apps = []*AppConfig{}
	}

	for i, existingApp := range apps {
		if existingApp.Name == app.Name {
			apps[i] = app
			return SaveApps(apps, filePath)
		}
	}

	apps = append(apps, app)
	return SaveApps(apps, filePath)
}

func SaveApps(apps []*AppConfig, filePath string) error {
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filePath, data, 0644)
}

func RemoveApp(appName, filePath string) error {
	apps, err := LoadApps(filePath)
	if err != nil {
		return err
	}

	var updatedApps []*AppConfig
	for _, app := range apps {
		if app.Name != appName {
			updatedApps = append(updatedApps, app)
		}
	}

	return SaveApps(updatedApps, filePath)
}

func LoadApps(filePath string) ([]*AppConfig, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var apps []*AppConfig
	if err := json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}
	return apps, nil
}
