# WTWatcher

WTWatcher is a robust, decoupled agent-based network monitoring application that tracks network latency, packet loss, and bandwidth throughput over time. It features a lightweight Go backend with an asynchronous task scheduler and a clean, responsive TypeScript/TailwindCSS frontend dashboard.

# TO FIX!!

1. Charts time axis needs to be decoupled speedtest ran at 18:30:16 and ping ran at 18:30:00 you get a 18:30:16 empty point on the latency chart.
2. Speedtest chart download and upload colours don't match the speedtest card.

## Features

- **Ping Agent (Latency Tracking):** Continuously monitors ICMP latency and packet loss against configured endpoints (supports both IPv4 and IPv6). Automatically handles privilege escalation and fallback.
- **Speedtest Agent (Bandwidth Tracking):** Wraps the official Ookla Speedtest CLI to evaluate ISP performance and measure maximum download/upload throughput.
- **Archive Agent (Data Retention):** Automatically rotates and archives historical metrics to prevent infinite file growth, ensuring quick load times on the frontend.
- **Web Dashboard:** A responsive, dynamically themed dashboard built with TypeScript, TailwindCSS, DaisyUI, and Chart.js.
- **Customizable Scheduler:** Configure precise intervals for ping tests, speedtests, and archiving via a straightforward `config.yml` file.

## Tech Stack

- **Backend:** Go (Golang)
- **Frontend:** TypeScript, HTML, CSS (TailwindCSS, DaisyUI)
- **Libraries:** Chart.js, Luxon, `prometheus-community/pro-bing`, `gopkg.in/yaml.v3`
- **External CLI:** Ookla Speedtest CLI

## Installation & Prerequisites

1. **Go:** Ensure you have Go installed on your system.
2. **Node.js & npm:** Required for compiling frontend assets.
3. **Ookla Speedtest CLI:** (Optional) Required if you wish to enable the Speedtest Agent. Follow the [official instructions](https://www.speedtest.net/apps/cli) to install it on your system.

### Build the Project

First, install the frontend dependencies and build the static assets:

```bash
npm install
npm run build
```

*(Note: During development, you can use `npm run watch` to automatically rebuild assets as you edit the UI source files).*

## Configuration

WTWatcher uses a `config.yml` file to manage its scheduling and target addresses. An example configuration:

```yaml
Schedule:
    Ping: 5 Minutes # Minutes or Hours
    Speedtest: OFF # Minutes, Hours or OFF (official Ookla Speedtest CLI required)
    Archiving: 14 Days # Minutes, Hours or Days

Addresses:
    Gateway:
        IPv4: 192.168.1.1
    Cloudflare DNS:
        IPv6: 2606:4700:4700::1111
        IPv4: 1.1.1.1
    Youtube:
        Domain: youtube.com
        Protocol: Both # IPv4, IPv6 or Both
```

## Running the Application

To start the background monitoring agents along with the built-in HTTP server:

```bash
go run main.go -server
```

By default, the server will start on port `8080`. You can view your dashboard at `http://localhost:8080`.

**Other Command Line Options:**

- `go run main.go`: Starts only the background monitoring agents (Scheduler, Ping, Speedtest, Archiving) without starting the web server.
- `go run main.go -config path/to/config.yml`: Uses a specific configuration file.

## Architecture Details

See `AGENTS.md` for a comprehensive overview of the agent architecture, directory structure, and responsibilities of each module.
