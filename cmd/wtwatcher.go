package cmd

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/chrisriddell/wtwatcher/cmd/modules"
	"github.com/chrisriddell/wtwatcher/public"
)

// Run is the entry point called from main.go.
func Run() {
	// CLI flags
	serverFlag := flag.Bool("server", false, "Start the HTTP server to serve public/ files")
	portFlag := flag.Int("port", 8080, "Port for the HTTP server (default: 8080)")
	configFlag := flag.String("config", "config.yml", "Path to the configuration file")
	flag.Parse()

	// Bootstrap required files and folders
	if err := bootstrap(*configFlag); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap error: %v\n", err)
		os.Exit(1)
	}

	// Initialise logger
	logger, err := modules.NewLogger("./log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Load & validate config
	cfg, err := modules.LoadConfig(*configFlag)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	logger.Info("configuration loaded successfully", "config", *configFlag)

	// Initialise file manager (metrics.json in ./public/)
	fm, err := modules.NewFileManager("./public/metrics.json", "./archive")
	if err != nil {
		logger.Error("failed to initialise file manager", "error", err)
		os.Exit(1)
	}

	// Initialise and start scheduler
	scheduler := modules.NewScheduler(cfg, fm, logger)
	scheduler.Start()
	logger.Info("scheduler started")

	// Optionally start HTTP server
	if *serverFlag {
		srv := modules.NewServer(*portFlag, "./public", logger)
		go func() {
			if err := srv.Start(); err != nil {
				logger.Error("server error", "error", err)
			}
		}()
	}

	// Block until SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down…")
	logger.Info("shutdown signal received")
	scheduler.Stop()
	logger.Info("scheduler stopped cleanly")
}

// bootstrap ensures that required directories and the configuration file exist.
func bootstrap(configPath string) error {
	dirs := []string{"./log", "./public", "./archive"}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %v", dir, err)
			}
		}
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := `---
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
`
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return fmt.Errorf("failed to create default config file %s: %v", configPath, err)
		}
	}

	// Create frontend files if they are missing
	frontendFiles := map[string][]byte{
		"./public/index.html": public.IndexHTML,
		"./public/styles.css": public.StylesCSS,
		"./public/scripts.js": public.ScriptsJS,
	}
	for path, content := range frontendFiles {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, content, 0644); err != nil {
				return fmt.Errorf("failed to create %s: %v", path, err)
			}
		}
	}

	return nil
}
