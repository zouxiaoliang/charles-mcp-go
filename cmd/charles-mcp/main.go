package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zouxiaoliang/charles-mcp-go/internal/app"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "charles-mcp:", err)
		os.Exit(1)
	}
}
func run() error {
	appVersion := applicationVersion()
	config := flag.String("config", "", "JSON configuration file")
	dataDir := flag.String("data-dir", "", "capture database directory")
	check := flag.Bool("check", false, "check Charles connectivity and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	tools := flag.Bool("tools", false, "print tool schemas and exit")
	legacy := flag.Bool("legacy-aliases", false, "enable deprecated tool aliases")
	flag.Parse()
	if *showVersion {
		fmt.Println(appVersion)
		return nil
	}
	if *tools {
		defs, err := app.ToolDefinitions(*legacy)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(defs)
	}
	cfg, err := app.LoadConfig(*config)
	if err != nil {
		return err
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
		if os.Getenv("CHARLES_RECORDINGS_DIR") == "" {
			cfg.RecordingsDir = *dataDir + string(os.PathSeparator) + "recordings"
		}
	}
	if *legacy {
		cfg.LegacyAliases = true
	}
	a, err := app.New(cfg)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.Close(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *check {
		r, err := a.Call(ctx, "charles_status", app.Args{})
		if err != nil {
			return err
		}
		if err = json.NewEncoder(os.Stdout).Encode(r); err != nil {
			return err
		}
		if !r.(map[string]any)["connected"].(bool) {
			return fmt.Errorf("Charles connectivity check failed")
		}
		return nil
	}
	server, err := a.Server(appVersion)
	if err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := a.Live.Expire(ctx); err != nil {
					log.Printf("expire live sessions: %v", err)
				}
			}
		}
	}()
	err = server.Run(ctx, &mcp.StdioTransport{})
	if ctx.Err() != nil {
		return nil
	}
	return err
}
