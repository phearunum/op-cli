# Op: A Simple Go Process Manager

**Op** is a lightweight and minimalistic process manager built in Go. It's designed for easily managing, monitoring, and controlling your applications as background services. With a simple command-line interface, you can start, stop, and get real-time status of your running processes.

---

### Features

- **Daemonization:** Run your application as a background daemon.
- **Service Management:** Easily create, start, and stop managed services.
- **Real-time Monitoring:** View the status, uptime, CPU, and memory usage of all running services.
- **Network Monitoring:** Special support for network **ping monitoring** to track host availability.
- **Live Log Viewing:** Stream real-time logs (stdout/stderr) from your services.
- **Automatic Restart:** Services that fail or exit will be automatically restarted by the daemon.
- **Configuration:** Manage service configurations using a `pm_config.toml` file.

---

### Getting Started

#### Building from Source

To build the `op` command-line tool, navigate to the project root and run:

```bash
go install ./cmd

## Usage

help
A simple process manager built in Go, with features like daemonization, process control, and monitoring.

Usage:
op [flags]
op [command]

Available Commands:
all List all services and ping statuses in separate tables.
completion Generate the autocompletion script for the specified shell
create Create and start a new service.
help Help about any command
init Create a sample 'pm_config.toml' file.
ls List all managed services in real-time.
ping Manages network ping services.
ps List all running processes on the system (raw output).
start Start the process manager daemon.
stop Stop the process manager daemon.
stop Stop a specific managed service.

Flags:
-h, --help help for op

Use "op [command] --help" for more information about a command.

## Create Ping

op ping create 192.168.1.1
op ping ls

## Service Management

op create web-server --cmd "node example/node/app.js" --port 3001
op ls

## Dashborad

op all
```
