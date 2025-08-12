package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ops-service/config"
	"ops-service/manager"

	"github.com/common-nighthawk/go-figure"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/sevlyar/go-daemon"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/spf13/cobra"
)

var (
	pidFile    = "/tmp/op_daemon.pid"
	logFile    = "/tmp/op_daemon.log"
	socketPath = "/tmp/op_daemon.sock"
)

var rootCmd = &cobra.Command{
	Use:   "op",
	Short: "A simple process manager similar to PM2.",
	Long:  `A simple process manager built in Go, with features like daemonization, process control, and monitoring.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all managed services in real-time.",
	Run: func(cmd *cobra.Command, args []string) {
		runLiveTable(manager.CmdStatus)
	},
}

var createCmd = &cobra.Command{
	Use:   "create <service_name>",
	Short: "Create and start a new service.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		serviceName := args[0]
		serviceCmd, _ := cmd.Flags().GetString("cmd")
		servicePort, _ := cmd.Flags().GetInt("port")

		if serviceCmd == "" {
			log.Fatal("The --cmd flag is required.")
		}

		appConfig := manager.AppConfig{
			Name:    serviceName,
			Command: serviceCmd,
			Port:    servicePort,
		}
		payload, _ := json.Marshal(appConfig)

		if err := sendIPCCommandAndPrint(manager.CmdCreate, string(payload)); err != nil {
			log.Fatalf("Failed to create service: %v", err)
		}
	},
}

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Manages network ping services.",
}

var pingCreateCmd = &cobra.Command{
	Use:   "create <ip_address>",
	Short: "Creates a new network ping service to monitor an IP address.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ipAddress := args[0]
		payload, _ := json.Marshal(map[string]string{"ip": ipAddress})
		if err := sendIPCCommandAndPrint(manager.CmdCreatePing, string(payload)); err != nil {
			log.Fatalf("Failed to create ping service: %v", err)
		}
	},
}

var pingLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all managed ping services in real-time.",
	Run: func(cmd *cobra.Command, args []string) {
		runLiveTable(manager.CmdPingStatus)
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a sample 'pm_config.toml' file.",
	Run: func(cmd *cobra.Command, args []string) {
		config.WriteSampleConfig()
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the process manager daemon.",
	Run: func(cmd *cobra.Command, args []string) {
		cntxt := &daemon.Context{
			PidFileName: pidFile,
			LogFileName: logFile,
			WorkDir:     "./",
			Umask:       027,
		}

		d, err := cntxt.Reborn()
		if err != nil {
			log.Fatalf("Unable to start daemon: %v", err)
		}
		if d != nil {
			fmt.Println("Process manager daemon started in the background.")
			return
		}
		defer cntxt.Release()

		log.Println("Daemon is running...")

		processManager := manager.NewManager()

		cfg, err := config.LoadConfig("pm_config.toml")
		if err == nil {
			for _, appCfg := range cfg.Apps {
				processManager.AddProcess(appCfg)
			}
		}

		apps, err := manager.LoadApps(manager.ListFile)
		if err == nil {
			for _, appCfg := range apps {
				processManager.AddProcess(appCfg)
			}
		}

		processManager.StartAll()

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			sig := <-sigs
			log.Printf("Received signal: %v. Shutting down gracefully...", sig)
			processManager.Shutdown()
			os.Exit(0)
		}()

		processManager.RunAsDaemon()
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the process manager daemon.",
	Run: func(cmd *cobra.Command, args []string) {
		cntxt := &daemon.Context{
			PidFileName: pidFile,
		}

		d, err := cntxt.Search()
		if err != nil {
			fmt.Printf("Daemon is not running: %v\n", err)
			return
		}

		if d == nil {
			fmt.Println("Daemon process not found, but PID file exists. Cleaning up.")
			if err := cntxt.Release(); err != nil {
				log.Printf("Failed to release PID file: %v", err)
			}
			os.Remove(socketPath)
			return
		}

		if err := d.Signal(syscall.SIGTERM); err != nil {
			log.Fatalf("Failed to send termination signal: %v", err)
		}
		fmt.Println("Sent stop signal to the daemon.")
	},
}

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List all running processes on the system (raw output).",
	Run: func(cmd *cobra.Command, args []string) {
		processes, err := process.Processes()
		if err != nil {
			log.Fatalf("Failed to get processes: %v", err)
		}

		for _, p := range processes {
			name, err := p.Name()
			if err != nil {
				name = "N/A"
			}
			status, err := p.Status()
			if err != nil || len(status) == 0 {
				status = []string{"N/A"}
			}
			cpu, err := p.CPUPercent()
			cpuStr := "N/A"
			if err == nil {
				cpuStr = fmt.Sprintf("%.1f%%", cpu)
			}
			mem, err := p.MemoryInfo()
			memStr := "N/A"
			if err == nil {
				memStr = byteSize(mem.RSS)
			}

			// Print without table headers
			fmt.Printf("%d %s %s %s %s\n",
				p.Pid,
				name,
				cpuStr,
				memStr,
				strings.Join(status, ","),
			)
		}
	},
}

var stopServiceCmd = &cobra.Command{
	Use:   "stop <service_name>",
	Short: "Stop a specific managed service.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		serviceName := args[0]
		if err := sendIPCCommandAndPrint(manager.CmdStop, serviceName); err != nil {
			log.Fatalf("Failed to stop service: %v", err)
		}
	},
}

var allCmd = &cobra.Command{
	Use:   "all",
	Short: "List all services and ping statuses in separate tables.",
	Run: func(cmd *cobra.Command, args []string) {
		runAllLiveTables()
	},
}

func runLiveTable(command string) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-interrupt:
			fmt.Println("\nExiting real-time view.")
			return
		default:
			infos, err := getInfosFromDaemon(command)
			if err != nil {
				fmt.Printf("Error fetching data: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}

			fmt.Print("\033[H\033[J")
			fig := figure.NewColorFigure("OP Monitor", "banner3-D", "green", true)
			fig.Print()
			fmt.Println()

			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.SetStyle(table.StyleLight)

			if strings.HasPrefix(command, manager.CmdPingStatus) {
				t.SetTitle("Ping Services")
				t.AppendHeader(table.Row{"APP NAME", "PID", "STATUS", "PING STATUS", "UPTIME", "CPU", "MEMORY", "PORT"})
			} else {
				t.SetTitle("Managed Services")
				t.AppendHeader(table.Row{"APP NAME", "PID", "STATUS", "UPTIME", "CPU", "MEMORY", "PORT"})
			}

			for _, info := range infos {
				t.AppendRow(infoToRow(info, command))
			}

			t.Render()
			time.Sleep(1 * time.Second)
		}
	}
}

func runAllLiveTables() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-interrupt:
			fmt.Println("\nExiting real-time view.")
			return
		default:
			fmt.Print("\033[H\033[J")
			fig := figure.NewColorFigure("OP Monitor", "banner3-D", "green", true)
			fig.Print()
			fmt.Println()

			infos, err := getInfosFromDaemon(manager.CmdStatus)
			if err != nil {
				fmt.Printf("Error fetching service data: %v\n", err)
			} else {
				t := table.NewWriter()
				t.SetOutputMirror(os.Stdout)
				t.SetStyle(table.StyleLight)
				t.SetTitle("Managed Services")
				t.AppendHeader(table.Row{"APP NAME", "PID", "STATUS", "UPTIME", "CPU", "MEMORY", "PORT"})

				for _, info := range infos {
					t.AppendRow(infoToRow(info, manager.CmdStatus))
				}
				t.Render()
				fmt.Println()
			}

			pingInfos, err := getInfosFromDaemon(manager.CmdPingStatus)
			if err != nil {
				fmt.Printf("Error fetching ping data: %v\n", err)
			} else {
				pt := table.NewWriter()
				pt.SetOutputMirror(os.Stdout)
				pt.SetStyle(table.StyleLight)
				pt.SetTitle("Ping Services")
				pt.AppendHeader(table.Row{"APP NAME", "PID", "STATUS", "PING STATUS", "UPTIME", "CPU", "MEMORY", "PORT"})

				for _, info := range pingInfos {
					pt.AppendRow(infoToRow(info, manager.CmdPingStatus))
				}
				pt.Render()
			}

			time.Sleep(1 * time.Second)
		}
	}
}

func infoToRow(info *manager.ProcessInfo, command string) table.Row {
	statusColor := text.FgHiRed
	status := info.Status
	if info.Status == "running" {
		statusColor = text.FgHiGreen
	} else if info.Status == "exited" {
		statusColor = text.FgHiYellow
	}

	if info.Message != "" {
		status = fmt.Sprintf("%s (%s)", status, info.Message)
	}

	port := "N/A"
	if info.Port != 0 {
		port = fmt.Sprintf("%d", info.Port)
	}

	isPingTable := strings.HasPrefix(command, manager.CmdPingStatus)
	if isPingTable {
		pingStatusColor := text.FgHiRed
		if info.PingStatus == "OK" {
			pingStatusColor = text.FgHiGreen
		} else if info.PingStatus == "TIMEOUT" {
			pingStatusColor = text.FgHiRed
		}
		return table.Row{

			info.Name,
			info.PID,
			text.Colors{statusColor}.Sprintf("%s", status),
			text.Colors{pingStatusColor}.Sprintf("%s", info.PingStatus),
			info.Uptime,
			fmt.Sprintf("%.1f%%", info.CPU),
			byteSize(info.Memory),
			port,
		}
	} else {
		return table.Row{

			info.Name,
			info.PID,
			text.Colors{statusColor}.Sprintf("%s", status),
			info.Uptime,
			fmt.Sprintf("%.1f%%", info.CPU),
			byteSize(info.Memory),
			port,
		}
	}
}

func getInfosFromDaemon(command string) ([]*manager.ProcessInfo, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not connect to daemon: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	msg := manager.IPCMessage{Command: command, Payload: ""}
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(msg); err != nil {
		return nil, fmt.Errorf("failed to encode message: %v", err)
	}

	decoder := json.NewDecoder(conn)
	var infos []*manager.ProcessInfo
	if err := decoder.Decode(&infos); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %v", err)
	}
	return infos, nil
}

func sendIPCCommandAndPrint(command, payload string) error {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("could not connect to daemon: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	msg := manager.IPCMessage{Command: command, Payload: payload}
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(msg); err != nil {
		return fmt.Errorf("failed to encode message: %v", err)
	}

	var response interface{}
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}
	fmt.Println(response)

	return nil
}

func byteSize(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

func main() {
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().String("cmd", "", "The command to execute for the service.")
	createCmd.Flags().Int("port", 0, "The port to listen on for the service.")
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(daemonStopCmd)
	rootCmd.AddCommand(psCmd)
	rootCmd.AddCommand(pingCmd)
	pingCmd.AddCommand(pingCreateCmd)
	pingCmd.AddCommand(pingLsCmd)
	rootCmd.AddCommand(stopServiceCmd)
	rootCmd.AddCommand(allCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
