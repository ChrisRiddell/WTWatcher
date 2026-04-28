# WTWatcher Agent Architecture

WTWatcher is built upon a robust, decoupled agent-based architecture. This design allows for independent execution, precise scheduling, and graceful failure handling across various background tasks. This document details the core agents that make up the WTWatcher ecosystem.

## Architecture & Directory Structure

```
WTWatcher/
├── cmd/
│   ├── wtwatcher.go        # Core initialization and setup
│   └── modules/            # Backend functionality
│       ├── collector_ping.go      # Uses pro-bing for ICMP latency
│       ├── collector_speedtest.go # CLI-based speedtest execution
│       ├── config.go              # Parses config.yml
│       ├── file_management.go     # Metrics JSON writing and archiving
│       ├── logging.go             # Custom structured logging
│       ├── scheduler.go           # Task scheduler for interval-based execution
│       └── server.go              # Simple HTTP server for the dashboard
├── ui/                     # Frontend Source Code
│   └── scripts.ts          # Frontend TypeScript logic (Chart.js, Luxon)
├── public/                 # Compiled/Static Frontend Assets
│   ├── index.html          # Dashboard entry point
│   ├── styles.css          # CSS
│   └── scripts.js          # Compiled JS
├── archive/                # Historical metrics (rotated daily)
├── log/                    # Log files
├── main.go                 # Go application entry point
├── config.yml              # User configuration
├── package.json            # Node.js dependencies & npm scripts
└── go.mod                  # Go module dependencies
```

## Tech Stack Overview

*   **Backend:** Go (Golang)
*   **Frontend:** HTML, TypeScript, CSS
*   **UI Libraries:** Chart.js, Luxon
*   **Core Dependencies:** `prometheus-community/pro-bing` (ICMP checks), gopkg.in/yaml.v3 (YAML parsing)
*   **External CLI:** Ookla Speedtest CLI

## Commands & Workflows

### Frontend
Managed via `npm` scripts defined in `package.json`:
*   `npm run build`: Compiles the raw `ui/scripts.ts` into the static `public/` directory (`scripts.js`).
*   `npm run watch`: Runs the TypeScript compiler in parallel watch mode, automatically rebuilding assets when source files change.

### Backend
Executed via the `go run` command (or as a compiled binary):
*   `go run main.go`: Boots up the background monitoring agents (Scheduler, Ping, Speedtest, Archiving) without a web server.
*   `go run main.go -server`: Starts the monitoring agents **and** binds the built-in HTTP server to serve the frontend (default port 8080).
*   `go run main.go -config path/to/config.yml`: Runs the backend using a specific or custom configuration file path.

## Scheduler (`modules/scheduler.go`)

The Scheduler is the heart of the WTWatcher backend, acting as the primary orchestrator for all data collection and management tasks.

*   **Role:** Conductor of recurring background tasks.
*   **Responsibilities:**
    *   **Queue Management:** Manages an asynchronous queue of scheduled jobs (Ping, Speedtest, and Archiving).
    *   **Time Alignment:** Automatically aligns task execution times (using UTC) to strict clock intervals (e.g., triggering exactly on the minute or hour) based on the user's `config.yml`.
    *   **Concurrency Control:** Uses a single worker goroutine to process the queue, ensuring tasks do not overlap and overwhelm system resources.
    *   **Timeout Enforcement:** Wraps every task execution in a rigid 5-minute context timeout. If a child agent hangs, the Scheduler will cancel it, log an error, and proceed to the next item.

## Ping Agent (`modules/collector_ping.go`)

The Ping Agent is responsible for continuous tracking of network latency and connection stability against configured endpoints.

*   **Role:** ICMP latency and packet loss collector.
*   **Responsibilities:**
    *   **Target Resolution:** Dynamically resolves IP addresses and domain names. If a domain is configured with the protocol `Both`, the agent performs dual DNS lookups to collect metrics for both A (IPv4) and AAAA (IPv6) records.
    *   **Execution:** Utilizes the `prometheus-community/pro-bing` package to send repeated ICMP echo requests. 
    *   **Resilience:** Automatically detects OS-level privilege restrictions (common on macOS). It attempts a raw-socket privileged ping first; if denied, it instantly falls back to an unprivileged UDP-based ICMP ping to ensure metrics are still collected.
    *   **Data Formatting:** Averages the Round-Trip Time (RTT) into milliseconds, calculates any packet loss percentage, and passes the payload back to the File Manager.

## Speedtest Agent (`modules/collector_speedtest.go`)

The Speedtest Agent monitors the maximum throughput of the network connection to evaluate ISP performance.

*   **Role:** Bandwidth capacity evaluator.
*   **Responsibilities:**
    *   **CLI Integration:** Acts as a wrapper around the external Ookla `speedtest` CLI tool, utilizing the `--format=json` flag to parse results cleanly without screen scraping.
    *   **Data Conversion:** Extracts raw byte-per-second statistics from the CLI and calculates accurate Megabits-per-second (Mbps) for both Download and Upload streams, rounded to two decimal places.
    *   **Submission:** Forwards the sanitized result structure to the File Manager to be appended to the current time slot.

## Archive Agent (`modules/file_management.go`)

The Archive Agent ensures that the live application data doesn't grow infinitely, maintaining performance and organization for the frontend.

*   **Role:** Data retention and rotation manager.
*   **Responsibilities:**
    *   **Live Data Management:** Governs all thread-safe access to the primary `public/metrics.json` file. It relies on mutex locks to ensure the Ping and Speedtest agents don't corrupt the file when writing simultaneously.
    *   **Data Rotation:** Periodically evaluates all timestamps in the live metrics file against the configured retention limit (e.g., 14 days). 
    *   **Archiving:** Migrates expired time slots out of the live metrics file and merges them into historical day-partitioned files (e.g., `archive/2026-04-20.json`).
    *   **Safety:** Implements atomic write strategies (writing to a `.tmp` file and renaming) and auto-backup recovery to prevent data loss in the event of an abrupt power failure or crash during a file write.

## Web Server (`modules/server.go`)

The Web Server provides the delivery mechanism for the user interface and the underlying data.

*   **Role:** HTTP static file and data server.
*   **Responsibilities:**
    *   **Static Serving:** Hosts the compiled frontend application (`index.html`, CSS, JS) from the `./public` directory.
    *   **Data Exposure:** Makes the live `metrics.json` accessible to the frontend dashboard for parsing and visualization.
    *   **Configuration:** Listens on standard IPv4 loopback (`127.0.0.1`) using a user-configurable port (default: 8080) provided via command-line arguments.