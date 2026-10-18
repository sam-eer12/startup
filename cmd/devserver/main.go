// devserver runs a loopback-only Week 1 simulator/collector. Use the container
// profile for Linux development; it has no production ingress or durability API.
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

	"github.com/sam-eer12/startup/internal/devserver"
	"github.com/sam-eer12/startup/internal/storage"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8081", "listen address")
	objects := flag.String("objects", ".local/objects", "development redacted object directory")
	mode := flag.String("mode", "all", "all, collector or simulator")
	container := flag.Bool("container-local", false, "allow container network binding in local Compose only")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || (!*container && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback())) {
		slog.Error("local profile requires a loopback address")
		os.Exit(1)
	}
	store, err := storage.NewFilesystem(*objects)
	if err != nil {
		slog.Error("cannot initialize local object directory")
		os.Exit(1)
	}
	handler, err := devserver.New(store, *mode)
	if err != nil {
		slog.Error("invalid local fixture configuration")
		os.Exit(1)
	}
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := store.Prune(); err != nil {
		slog.Error("local retention sweep failed")
		os.Exit(1)
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.Prune(); err != nil {
					slog.Warn("local retention sweep failed")
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("synthetic local service listening", "address", *listen, "mode", *mode)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("local service failed", "error", err)
		os.Exit(1)
	}
}
