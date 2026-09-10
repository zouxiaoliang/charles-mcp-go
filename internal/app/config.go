package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	DataDir           string `json:"data_dir"`
	RecordingsDir     string `json:"recordings_dir"`
	CharlesURL        string `json:"charles_url"`
	ProxyURL          string `json:"proxy_url"`
	User              string `json:"user"`
	Password          string `json:"password"`
	CharlesCLI        string `json:"charles_cli"`
	TimeoutSeconds    int    `json:"timeout_seconds"`
	SessionTTLSeconds int    `json:"session_ttl_seconds"`
	LegacyAliases     bool   `json:"legacy_aliases"`
}

func LoadConfig(path string) (Config, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, err
	}
	c := Config{DataDir: filepath.Join(dir, "charles-mcp"), CharlesURL: "http://control.charles", ProxyURL: "http://127.0.0.1:8888", User: "admin", Password: "123456", TimeoutSeconds: 30, SessionTTLSeconds: 900}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return c, err
		}
		defer f.Close()
		dec := json.NewDecoder(f)
		dec.DisallowUnknownFields()
		if err = dec.Decode(&c); err != nil {
			return c, err
		}
	}
	for key, target := range map[string]*string{"CHARLES_MCP_DATA_DIR": &c.DataDir, "CHARLES_RECORDINGS_DIR": &c.RecordingsDir, "CHARLES_BASE_URL": &c.CharlesURL, "CHARLES_PROXY_URL": &c.ProxyURL, "CHARLES_USER": &c.User, "CHARLES_PASS": &c.Password, "CHARLES_CLI_PATH": &c.CharlesCLI} {
		if v, ok := os.LookupEnv(key); ok {
			*target = v
		}
	}
	if c.DataDir == "" {
		return c, fmt.Errorf("data_dir must not be empty")
	}
	c.DataDir, err = filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	if c.RecordingsDir == "" {
		c.RecordingsDir = filepath.Join(c.DataDir, "recordings")
	}
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 300 {
		return c, fmt.Errorf("timeout_seconds must be 1..300")
	}
	if time.Duration(c.SessionTTLSeconds)*time.Second < time.Second {
		return c, fmt.Errorf("session_ttl_seconds must be positive")
	}
	return c, nil
}
