// Package devserver supplies the Week 1 local fixture simulator and collector.
// It is deliberately separate from future authenticated production services.
package devserver

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sam-eer12/startup/connectors"
	"github.com/sam-eer12/startup/fixtures"
	"github.com/sam-eer12/startup/internal/capture"
	"github.com/sam-eer12/startup/internal/storage"
)

type Observation struct {
	SchemaVersion        int     `json:"schema_version"`
	ObservationID        string  `json:"observation_id"`
	TenantID             string  `json:"tenant_id"`
	EnvironmentID        string  `json:"environment_id"`
	RouteID              string  `json:"route_id"`
	ProviderVersion      string  `json:"provider_version"`
	EventFamily          string  `json:"event_family"`
	Direction            string  `json:"direction"`
	ObservedAt           string  `json:"observed_at"`
	ExpiresAt            string  `json:"expires_at"`
	CaptureQuality       string  `json:"capture_quality"`
	SampleProbability    float64 `json:"sample_probability"`
	Authoritative        bool    `json:"authoritative"`
	SignatureStatus      string  `json:"signature_status"`
	RedactionPolicy      string  `json:"redaction_policy"`
	SemanticVerification string  `json:"semantic_verification"`
	RedactedObjectRef    string  `json:"redacted_object_ref"`
}

type Server struct {
	store    *storage.Filesystem
	routes   map[string]connectors.Route
	accepted atomic.Uint64
	rejected atomic.Uint64
	dropped  atomic.Uint64
}

func New(store *storage.Filesystem, mode string) (http.Handler, error) {
	if mode != "all" && mode != "collector" && mode != "simulator" {
		return nil, errors.New("unknown server mode")
	}
	routes, err := connectors.Routes()
	if err != nil {
		return nil, err
	}
	s := &Server{store: store, routes: routes}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "profile": "synthetic-local"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "dependencies": "local-fixtures-only"})
	})
	mux.HandleFunc("GET /metrics", s.metrics)
	if mode != "collector" {
		mux.HandleFunc("GET /fixtures/{connector}/{scenario}", s.fixture)
		mux.HandleFunc("GET /runtime/{operation}", s.runtimeFixture)
	}
	if mode != "simulator" {
		mux.HandleFunc("POST /observe/{route}", s.observe)
	}
	return mux, nil
}

// runtimeFixture is an opt-in synthetic upstream for the Week 2 Envoy lab.
func (s *Server) runtimeFixture(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("operation") != "identity" && r.PathValue("operation") != "move" && r.PathValue("operation") != "bypass" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("case") == "empty" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", `"synthetic-original"`)
	w.Header().Set("Content-Digest", "synthetic-original-digest")
	if r.URL.Query().Get("case") == "malformed" {
		w.Write([]byte("{"))
		return
	}
	body, _ := fixtures.Files.ReadFile("synthetic/stripe/field-move.json")
	if requested := r.URL.Query().Get("size"); requested != "" {
		size := 0
		switch requested {
		case "32768":
			size = 32768
		case "262144":
			size = 262144
		default:
			http.Error(w, "unsupported synthetic size", 400)
			return
		}
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			http.Error(w, "fixture unavailable", 500)
			return
		}
		value["synthetic_padding"] = ""
		empty, _ := json.MarshalIndent(value, "", "  ")
		value["synthetic_padding"] = strings.Repeat("x", size-len(empty))
		body, _ = json.MarshalIndent(value, "", "  ")
	}
	if r.URL.Query().Get("case") == "chunked" {
		midpoint := len(body) / 2
		w.Write(body[:midpoint])
		w.(http.Flusher).Flush()
		w.Write(body[midpoint:])
		return
	}
	w.Write(body)
}

func (s *Server) fixture(w http.ResponseWriter, r *http.Request) {
	family, scenario := r.PathValue("connector"), r.PathValue("scenario")
	// No filesystem traversal, arbitrary filenames or golden/holdout access.
	if !safeSegment(family) || !safeSegment(scenario) {
		http.NotFound(w, r)
		return
	}
	body, err := fixtures.Files.ReadFile("synthetic/" + family + "/" + scenario + ".json")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Mesh-Fixture", "synthetic-unverified")
	if scenario == "auth" {
		w.WriteHeader(http.StatusUnauthorized)
	} else if scenario == "rate-limiting" {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}
	w.Write(body)
}

func safeSegment(v string) bool {
	if v == "" {
		return false
	}
	for _, c := range v {
		if (c < 'a' || c > 'z') && c != '-' {
			return false
		}
	}
	return true
}

func (s *Server) observe(w http.ResponseWriter, r *http.Request) {
	route, ok := s.routes[r.PathValue("route")]
	if !ok {
		s.reject(w, http.StatusNotFound, "unknown local route")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || (r.Header.Get("Content-Encoding") != "" && r.Header.Get("Content-Encoding") != "identity") {
		s.reject(w, http.StatusUnsupportedMediaType, "complete uncompressed application/json required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, capture.MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			s.reject(w, http.StatusRequestEntityTooLarge, "body exceeds local capture limit")
		} else {
			s.reject(w, http.StatusBadRequest, "incomplete body")
		}
		return
	}
	value, err := capture.Decode(body)
	if err != nil {
		s.reject(w, http.StatusUnprocessableEntity, "body is not eligible complete JSON")
		return
	}
	redacted, err := json.Marshal(capture.Redact(value))
	if err != nil {
		s.reject(w, http.StatusInternalServerError, "redaction failed")
		return
	}
	ref, err := s.store.Put(redacted)
	if err != nil {
		s.dropped.Add(1)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "local sample storage unavailable"})
		return
	}
	now := time.Now().UTC()
	observation := Observation{
		SchemaVersion: 1, ObservationID: "obs_" + newID(), TenantID: route.TenantID, EnvironmentID: route.EnvironmentID,
		RouteID: route.RouteID, ProviderVersion: route.ProviderVersion, EventFamily: route.EventFamily, Direction: route.Direction,
		ObservedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Duration(route.PayloadPolicy.TelemetryRetentionHours) * time.Hour).Format(time.RFC3339Nano),
		CaptureQuality: "complete", SampleProbability: 1, Authoritative: false, SignatureStatus: "unverified_fixture",
		RedactionPolicy: route.PayloadPolicy.RedactionPolicy, SemanticVerification: "unavailable_after_redaction", RedactedObjectRef: ref,
	}
	// Only the redacted object is persisted. No raw bytes, headers, URL query,
	// event identifiers or user-supplied metadata enter the observation.
	s.accepted.Add(1)
	writeJSON(w, http.StatusOK, observation)
}

func (s *Server) reject(w http.ResponseWriter, status int, message string) {
	s.rejected.Add(1)
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	// Fixed labels only: never use customer IDs or request paths as labels.
	io.WriteString(w, "# HELP mesh_capture_samples_total Local synthetic capture outcomes.\n# TYPE mesh_capture_samples_total counter\n")
	for _, counter := range []struct {
		label string
		value uint64
	}{{"captured", s.accepted.Load()}, {"rejected", s.rejected.Load()}, {"storage_failed", s.dropped.Load()}} {
		io.WriteString(w, "mesh_capture_samples_total{outcome=\""+counter.label+"\"} "+formatUint(counter.value)+"\n")
	}
	io.WriteString(w, "# HELP mesh_provider_signatures_verified Whether signature verification is implemented.\n# TYPE mesh_provider_signatures_verified gauge\nmesh_provider_signatures_verified 0\n")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
