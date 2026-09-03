# WTWatcher

WTWatcher is a robust, decoupled agent-based network monitoring application that tracks network latency, packet loss, and bandwidth throughput over time. It features a lightweight Go backend with an asynchronous task scheduler and a clean, responsive frontend dashboard.

## Features

* **Ping Agent (Latency Tracking):** Continuously monitors ICMP latency and packet loss against configured endpoints (supports both IPv4 and IPv6). Automatically handles privilege escalation and fallback.
* **Speedtest Agent (Bandwidth Tracking):** Wraps the official Ookla Speedtest CLI to evaluate ISP performance and measure maximum download/upload throughput.
* **Archive Agent (Data Retention):** Automatically rotates and archives historical metrics to prevent infinite file growth, ensuring quick load times on the frontend.
* **Web Dashboard:** A responsive, dynamically themed dashboard built with TypeScript and Chart.js.
* **Customizable Scheduler:** Configure precise intervals for ping tests, speedtests, and archiving via a straightforward `config.yml` file.

## Prerequisites

* **Ookla Speedtest CLI:** (Optional) Required if you wish to enable the Speedtest Agent. Follow the [official instructions](https://www.speedtest.net/apps/cli) to install it on your system.

## Installation

Download the binary for your operating system from [https://github.com/ChrisRiddell/WTWatcher/releases](https://github.com/ChrisRiddell/WTWatcher/releases).

## Configuration

WTWatcher uses a `config.yml` file to manage its scheduling and target addresses. It is automatically created when you run the application for the first time.

## Running the Application

To start the background monitoring agents along with the built-in HTTP server:

```bash
./WTWatcher-<systemtype> -server

```

By default, the server will start on port `8080`. You can view your dashboard at `http://localhost:8080`.

### Command Line Options

* `./WTWatcher-<systemtype>`: Starts only the background monitoring agents (Scheduler, Ping, Speedtest, Archiving) without starting the web server.
* `./WTWatcher-<systemtype> -server`: Starts the background monitoring agents alongside the HTTP web server.
* `./WTWatcher-<systemtype> -server --port 9090`: Overrides the default HTTP server port (e.g., runs on port `9090`).
* `./WTWatcher-<systemtype> -config path/to/config.yml`: Uses a specific configuration file.

---

## Development Setup

If you are developing or building WTWatcher from source, you will need to set up the runtime dependencies manually.

### Tech Stack

* **Backend:** Go (Golang)
* **Frontend:** TypeScript, HTML, CSS
* **Libraries:** Chart.js, Luxon, `prometheus-community/pro-bing`, `gopkg.in/yaml.v3`
* **External CLI:** Ookla Speedtest CLI

### Development Prerequisites

1. **Go:** Version 1.18 or higher.
2. **Node.js & npm:** Required for compiling frontend assets.

### Running in Development Mode

Run the backend directly using Go:

```bash
go run main.go -server
```
