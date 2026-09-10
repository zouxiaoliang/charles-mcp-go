package main

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStdioHelper(t *testing.T) {
	if os.Getenv("CHARLES_TEST_HELPER") != "1" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("charles-mcp", flag.ExitOnError)
	os.Args = []string{"charles-mcp", "--data-dir", os.Getenv("CHARLES_TEST_DATA_DIR")}
	main()
	os.Exit(0)
}
func TestStdioProcessHandshakeAndToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestStdioHelper$")
	cmd.Env = append(os.Environ(), "CHARLES_TEST_HELPER=1", "CHARLES_TEST_DATA_DIR="+t.TempDir())
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "integration-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := session.InitializeResult().ServerInfo.Version; got != applicationVersion() {
		t.Fatalf("server version = %q, want %q", got, applicationVersion())
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 32 {
		t.Fatalf("list tools: %v %v", tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "reverse_list_captures", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("tool call: %+v %v", result, err)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
}
