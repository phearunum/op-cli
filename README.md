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
