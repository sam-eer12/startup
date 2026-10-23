// control-lab is an opt-in loopback-only synthetic Week 3 registry process.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sam-eer12/startup/internal/control"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8082", "loopback listen address")
	dir := flag.String("registry", ".local/week3-registry", "single-writer synthetic registry directory")
	enginePath := flag.String("engine", "target/debug/contract-ir", "offline Rust contract tool path")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		slog.Error("loopback address required")
		os.Exit(1)
	}
	engine := control.Engine{Path: *enginePath}
	var probe any
	if engine.Call(context.Background(), map[string]any{"operation": "normalize", "document": map[string]any{}, "pointer": ""}, &probe) != nil {
		slog.Error("build the offline engine with make build")
		os.Exit(1)
	}
	registry, err := control.Open(*dir, engine)
	if err != nil {
		slog.Error("registry initialization failed")
		os.Exit(1)
	}
	defer registry.Close()
	server := &http.Server{Addr: *listen, Handler: registry.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("synthetic Week 3 registry listening", "address", *listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("control lab stopped")
		os.Exit(1)
	}
}
