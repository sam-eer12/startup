package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	// This first CLI intentionally has only local read-only diagnostics. Contract
	// import and production mutations follow their own authenticated API later.
	base := flag.String("url", "http://127.0.0.1:8081", "local development service URL")
	flag.Parse()
	if flag.NArg() != 1 || (flag.Arg(0) != "health" && flag.Arg(0) != "metrics") {
		fmt.Fprintln(os.Stderr, "usage: meshctl [-url URL] health|metrics")
		os.Exit(2)
	}
	path := "/healthz"
	if flag.Arg(0) == "metrics" {
		path = "/metrics"
	}
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(*base + path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "local service unavailable")
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "local diagnostic failed")
		os.Exit(1)
	}
	_, err = io.Copy(os.Stdout, io.LimitReader(response.Body, 64*1024))
	if err != nil {
		os.Exit(1)
	}
}
