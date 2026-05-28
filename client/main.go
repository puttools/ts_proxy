package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"ts/tlsutil"
)

const version = "v1.0.0"

type ClientConfig struct {
	Password string `json:"password"`
	Server   string `json:"server"`
	Local    string `json:"local"`
	CAFile   string `json:"ca_file"`
	SkipVerify bool `json:"skip_verify"`
	HeartbeatSec int `json:"heartbeat_sec"`
	LogLevel string `json:"log_level"`
}

func main() {
	configFile := flag.String("config", "client_config.json", "path to client config file (json)")
	flag.Parse()

	data, err := os.ReadFile(*configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read config failed:", err)
		os.Exit(1)
	}
	var cfg ClientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "parse config failed:", err)
		os.Exit(1)
	}

	// apply defaults
	if cfg.Server == "" {
		cfg.Server = "127.0.0.1:9000"
	}
	if cfg.Local == "" {
		cfg.Local = "127.0.0.1:8080"
	}
	if cfg.HeartbeatSec == 0 {
		cfg.HeartbeatSec = 30
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	logger := newLogger(cfg.LogLevel)
	logger.Info("ts-client starting", "version", version, "server", cfg.Server, "local", cfg.Local)

	var caPEM []byte
	if cfg.CAFile != "" {
		caPEM, err = os.ReadFile(cfg.CAFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	tlsCfg, err := tlsutil.ClientConfig(caPEM, cfg.SkipVerify)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	Connect(cfg.Server, cfg.Local, tlsCfg, time.Duration(cfg.HeartbeatSec)*time.Second, logger, cfg.Password)
}

func newLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
