package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sam-eer12/startup/internal/devserver"
	"github.com/sam-eer12/startup/internal/storage"
)

func TestLocalFixtureToRedactedObservation(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewFilesystem(dir)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := devserver.New(store, "all")
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range []string{"stripe", "shopify", "netsuite", "generic-json"} {
		for _, scenario := range []string{"baseline", "field-move", "required-deletion"} {
			fixture := httptest.NewRecorder()
			handler.ServeHTTP(fixture, httptest.NewRequest(http.MethodGet, "/fixtures/"+family+"/"+scenario, nil))
			if fixture.Code != 200 {
				t.Fatalf("fixture missing: %s %s", family, scenario)
			}
			request := httptest.NewRequest(http.MethodPost, "/observe/route_"+family+"?token=QUERY_SECRET", strings.NewReader(fixture.Body.String()))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer HEADER_SECRET")
			request.Header.Set("X-Mesh-Tenant-Id", "tenant_attacker")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != 200 {
				t.Fatalf("capture failed: %s", recorder.Body.String())
			}
			var observation devserver.Observation
			if err := json.Unmarshal(recorder.Body.Bytes(), &observation); err != nil {
				t.Fatal(err)
			}
			if observation.Authoritative || observation.TenantID != "tenant_local" || observation.SignatureStatus != "unverified_fixture" || observation.CaptureQuality != "complete" {
				t.Fatal("observation crossed authority boundary")
			}
			path := filepath.Join(dir, strings.TrimPrefix(observation.RedactedObjectRef, "sha256:")+".json")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"HEADER_SECRET", "QUERY_SECRET", "synthetic@example.invalid", "TRACK-0001", "9007199254740993", "tenant_attacker"} {
				if strings.Contains(string(body)+recorder.Body.String(), secret) {
					t.Fatal("sample leaked secret")
				}
			}
		}
	}
}

type interruptedReader struct{}

func (interruptedReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestInterruptedBodyAndStorageFailure(t *testing.T) {
	dir := t.TempDir()
	store, _ := storage.NewFilesystem(dir)
	handler, _ := devserver.New(store, "collector")
	request := httptest.NewRequest("POST", "/observe/route_stripe", io.MultiReader(strings.NewReader(`{}`), interruptedReader{}))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 400 {
		t.Fatalf("interrupted body got %d", recorder.Code)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("partial body persisted")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest("POST", "/observe/route_stripe", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 503 {
		t.Fatal("storage failure acknowledged a sample")
	}
	metrics := httptest.NewRecorder()
	handler.ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `outcome="storage_failed"} 1`) {
		t.Fatal("storage failure is invisible")
	}
}

func TestRejectionsDoNotPersistEvidence(t *testing.T) {
	dir := t.TempDir()
	store, _ := storage.NewFilesystem(dir)
	handler, _ := devserver.New(store, "all")
	for _, tc := range []struct {
		body, contentType, encoding, route string
		status                             int
	}{
		{`{"x":1,"x":2}`, "application/json", "", "route_stripe", 422},
		{`{"x":`, "application/json", "", "route_stripe", 422},
		{`{}`, "text/plain", "", "route_stripe", 415},
		{`{}`, "application/json", "gzip", "route_stripe", 415},
		{`{}`, "application/json", "", "route_untrusted", 404},
		{`"` + strings.Repeat("x", 256*1024) + `"`, "application/json", "", "route_stripe", 413},
	} {
		request := httptest.NewRequest("POST", "/observe/"+tc.route, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", tc.contentType)
		request.Header.Set("Content-Encoding", tc.encoding)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("got %d want %d", response.Code, tc.status)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("ineligible evidence persisted")
	}
}

func TestSimulatorOperationalStatusAndRoleSeparation(t *testing.T) {
	store, _ := storage.NewFilesystem(t.TempDir())
	handler, _ := devserver.New(store, "simulator")
	for _, tc := range []struct {
		path   string
		status int
	}{{"/fixtures/generic-json/auth", 401}, {"/fixtures/generic-json/rate-limiting", 429}, {"/observe/route_stripe", 404}, {"/fixtures/stripe/unknown", 404}} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", tc.path, nil))
		if recorder.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, recorder.Code)
		}
	}
	collector, _ := devserver.New(store, "collector")
	recorder := httptest.NewRecorder()
	collector.ServeHTTP(recorder, httptest.NewRequest("GET", "/fixtures/stripe/baseline", nil))
	if recorder.Code != 404 {
		t.Fatal("collector exposed provider simulator")
	}
}
