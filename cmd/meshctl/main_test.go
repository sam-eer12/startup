package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandsUseExplicitFilesAndEscapedIncidentIDs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "request.json")
	if err := os.WriteFile(file, []byte(`{"route_id":"route_stripe"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args         []string
		method, path string
	}{{[]string{"health"}, "GET", "/healthz"}, {[]string{"incidents"}, "GET", "/v1/incidents"}, {[]string{"import", file}, "POST", "/v1/contracts"}, {[]string{"bind", file}, "POST", "/v1/bindings"}, {[]string{"drift", file}, "POST", "/v1/drift"}, {[]string{"review", "inc_1", file}, "POST", "/v1/incidents/inc_1/mapping-review"}} {
		method, path, _, err := command(tc.args)
		if err != nil || method != tc.method || path != tc.path {
			t.Fatalf("unexpected command result: %s %s %v", method, path, err)
		}
	}
	if _, _, _, err := command([]string{"import", filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("missing input accepted")
	}
	if _, _, _, err := command([]string{"deploy"}); err == nil {
		t.Fatal("unsupported mutation accepted")
	}
}
