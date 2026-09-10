package core

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestEnsureStopped(t *testing.T) {
	for _, useProxy := range []bool{false, true} {
		name := "direct"
		if useProxy {
			name = "proxy"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			endpoint := "http://" + listener.Addr().String()
			base, proxy := endpoint, ""
			if useProxy {
				base, proxy = "http://control.charles", endpoint
			}
			c, err := NewCharles(base, proxy, "", "", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if err := c.EnsureStopped(context.Background()); err == nil {
				t.Fatal("listening endpoint accepted as stopped")
			}
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
			if err := c.EnsureStopped(context.Background()); err != nil {
				t.Fatalf("closed endpoint not recognized: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := c.EnsureStopped(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled probe must not confirm shutdown: %v", err)
			}
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			if err := c.EnsureStopped(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("timed out probe must not confirm shutdown: %v", err)
			}
		})
	}
}
