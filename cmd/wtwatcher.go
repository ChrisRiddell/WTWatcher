package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chrisriddell/wtwatcher/cmd/modules"
	"github.com/chrisriddell/wtwatcher/public"
)

// Run is the main application coordinator. It parses CLI arguments, bootstraps
// required filesystem structures, initializes logging, reads configuration, sets up
// data storage and task scheduling, and manages graceful shutdown on OS termination signals.
func Run() {
	// Parse CLI flags for optional server mode, HTTP port, and custom config file path.
	serverFlag := flag.Bool("server", false, "Start the HTTP server to serve public/ files")
	portFlag := flag.Int("port", 8080, "Port for the HTTP server (default: 8080)")
	configFlag := flag.String("config", "config.yml", "Path to the configuration file")
	flag.Parse()

	// Bootstrap required directories (log, public, archive) and extract default
	// configuration and embedded frontend files if they do not yet exist on disk.
	if err := bootstrap(*configFlag); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap error: %v\n", err)
		os.Exit(1)
	}

	// Initialize multi-file structured logger (info.log, warning.log, error.log).
	logger, err := modules.NewLogger("./log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Load and validate user configuration from the specified YAML file.
	cfg, err := modules.LoadConfig(*configFlag)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	logger.Info("configuration loaded successfully", "config", *configFlag)

	// Initialize thread-safe file manager for reading and writing metrics.json and archiving.
	fm, err := modules.NewFileManager("./public/metrics.json", "./archive", logger)
	if err != nil {
		logger.Error("failed to initialise file manager", "error", err)
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Initialize and launch the periodic task scheduler (pings, speedtests, archiving).
	scheduler := modules.NewScheduler(cfg, fm, logger)
	scheduler.Start()
	logger.Info("scheduler started")

	// If the -server flag was provided, run the embedded HTTP server in a background goroutine.
	var srv *modules.Server
	if *serverFlag {
		srv = modules.NewServer(*portFlag, "./public", logger)
		go func() {
			if err := srv.Start(); err != nil {
				logger.Error("server error", "error", err)
			}
		}()
	}

	// Block main goroutine until an interrupt (Ctrl+C) or termination signal is received from the OS.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Begin graceful shutdown sequence to ensure in-flight tasks and file writes finish cleanly.
	fmt.Println("\nShutting down…")
	logger.Info("shutdown signal received")
	scheduler.Stop()
	logger.Info("scheduler stopped cleanly")

	// Gracefully shut down HTTP server with a 5-second deadline if running.
	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown error", "error", err)
		} else {
			logger.Info("server stopped cleanly")
		}
	}
}

// bootstrap ensures that required directories (log, public, archive) exist and
// extracts the default configuration and embedded frontend UI assets if they are missing.
func bootstrap(configPath string) error {
	// Create required directories with read/write/execute permissions.
	dirs := []string{"./log", "./public", "./archive"}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Generate a sensible default configuration file if one does not exist.
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		defaultConfig := `---
Schedule:
  Ping: 15 Minutes # Minutes or Hours
  Speedtest: OFF # Hours or OFF (official Ookla Speedtest CLI required)
  Archiving: 14 Days # Days or Months
  LogRotation: 7 Days # Days, Months or OFF

Ping:
  Count: 4 # Number of ICMP packets sent per probe cycle.
  Timeout: 10 Seconds # Overall timeout for completing a probe.
  Retries: 2 # Max retry attempts on probe failure before giving up.
  AnomalyThresholdMs: 2000 # Max realistic ping (ms). Spikes above this are filtered as anomalies.

Speedtest:
  ServerID: AUTO #Specify a server from the server list using its id (Speedtest --servers), AUTO for default

Addresses:
  Gateway:
    IPv4: 192.168.2.1
  Cloudflare DNS:
    IPv4: 1.1.1.1
    IPv6: 2606:4700:4700::1111
  Google DNS:
    IPv4: 8.8.8.8
    IPv6: 2001:4860:4860::8888
  Sydney:
    IPv4: 3.24.0.0
    IPv6: 2406:da1c:16:9000::ec2
  US West:
    IPv4: 13.52.0.0
    IPv6: 2600:1f1c:f64:3200::ec2
  US East:
    IPv4: 3.80.0.0
    IPv6: 2600:1f18:2fe:900::ec2
  Singapore:
    IPv4: 3.0.0.9
    IPv6: 2406:da18:ec6:6400::ec2
  London:
    IPv4: 3.8.0.0
    IPv6: 2a05:d01c:810:8100::ec2
  Tokyo:
    IPv4: 3.112.0.0
    IPv6: 2406:da14:295:300::ec2
  # Google:
  #   Domain: google.com
  #   Protocol: Both # IPv4, IPv6 or Both
`
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return fmt.Errorf("failed to create default config file %s: %w", configPath, err)
		}
	}

	// Write embedded frontend assets to the public/ folder if they don't already exist.
	// This enables the static HTTP server to serve the UI immediately out-of-the-box.
	frontendFiles := map[string][]byte{
		"./public/index.html": public.IndexHTML,
		"./public/styles.css": public.StylesCSS,
		"./public/scripts.js": public.ScriptsJS,
	}
	for path, content := range frontendFiles {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(path, content, 0644); err != nil {
				return fmt.Errorf("failed to create %s: %w", path, err)
			}
		}
	}

	return nil
}
