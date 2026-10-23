package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	base := flag.String("url", "http://127.0.0.1:8081", "local development service URL")
	flag.Parse()
	method, path, body, err := command(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	client := http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequest(method, strings.TrimRight(*base, "/")+path, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid local service URL")
		os.Exit(2)
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		fmt.Fprintln(os.Stderr, "local service unavailable")
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		fmt.Fprintln(os.Stderr, "local operation failed:", response.StatusCode)
		_, _ = io.Copy(os.Stderr, io.LimitReader(response.Body, 4096))
		os.Exit(1)
	}
	_, err = io.Copy(os.Stdout, io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		os.Exit(1)
	}
}

func command(args []string) (string, string, []byte, error) {
	usage := fmt.Errorf("usage: meshctl [-url URL] health|metrics|contracts|bindings|incidents|incident ID|import FILE|bind FILE|drift FILE|review ID FILE")
	if len(args) == 0 {
		return "", "", nil, usage
	}
	if len(args) == 1 {
		paths := map[string]string{"health": "/healthz", "metrics": "/metrics", "contracts": "/v1/contracts", "bindings": "/v1/bindings", "incidents": "/v1/incidents"}
		if path, ok := paths[args[0]]; ok {
			return "GET", path, nil, nil
		}
	}
	if len(args) == 2 && args[0] == "incident" {
		return "GET", "/v1/incidents/" + url.PathEscape(args[1]), nil, nil
	}
	path, file := "", ""
	if len(args) == 2 {
		path = map[string]string{"import": "/v1/contracts", "bind": "/v1/bindings", "drift": "/v1/drift"}[args[0]]
		file = args[1]
	}
	if len(args) == 3 && args[0] == "review" {
		path = "/v1/incidents/" + url.PathEscape(args[1]) + "/mapping-review"
		file = args[2]
	}
	if path == "" {
		return "", "", nil, usage
	}
	input, err := os.Open(file)
	if err != nil {
		return "", "", nil, fmt.Errorf("cannot open request file")
	}
	defer input.Close()
	body, err := io.ReadAll(io.LimitReader(input, 256*1024+1))
	if err != nil || len(body) > 256*1024 || !json.Valid(body) {
		return "", "", nil, fmt.Errorf("request file must be bounded JSON")
	}
	return "POST", path, body, nil
}
