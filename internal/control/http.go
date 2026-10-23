package control

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sam-eer12/startup/internal/capture"
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)

func (r *Registry) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, req *http.Request) {
		respond(w, 200, map[string]string{"profile": "synthetic-week3"})
	})
	mux.HandleFunc("POST /v1/contracts", r.importContract)
	mux.HandleFunc("GET /v1/contracts", r.listContracts)
	mux.HandleFunc("POST /v1/bindings", r.bind)
	mux.HandleFunc("GET /v1/bindings", r.listBindings)
	mux.HandleFunc("POST /v1/drift", r.drift)
	mux.HandleFunc("GET /v1/incidents", r.listIncidents)
	mux.HandleFunc("GET /v1/incidents/{id}", r.incident)
	mux.HandleFunc("POST /v1/incidents/{id}/mapping-review", r.review)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Browser-origin requests cannot mutate this unauthenticated loopback lab.
		if req.Header.Get("Origin") != "" {
			fail(w, 403, "browser origins disabled")
			return
		}
		if req.Method == http.MethodPost && req.Header.Get("Content-Type") != "application/json" {
			fail(w, 415, "application/json required")
			return
		}
		if !r.mu.TryLock() {
			fail(w, 503, "registry busy; retry later")
			return
		}
		defer r.mu.Unlock()
		mux.ServeHTTP(w, req)
	})
}

func readRequest(w http.ResponseWriter, req *http.Request, target any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, capture.MaxBodyBytes))
	if err != nil {
		fail(w, 413, "request body unavailable or exceeds limit")
		return false
	}
	if _, err := capture.Decode(body); err != nil {
		fail(w, 400, "invalid bounded JSON request")
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		fail(w, 400, "invalid request fields")
		return false
	}
	return true
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

func (r *Registry) importContract(w http.ResponseWriter, req *http.Request) {
	var input struct {
		Contract    Contract `json:"contract"`
		Pointer     string   `json:"pointer"`
		SourceLabel string   `json:"source_label"`
	}
	if !readRequest(w, req, &input) {
		return
	}
	c := input.Contract
	if c.SchemaVersion != 1 || c.TenantID != "tenant_local" || c.EnvironmentID != "env_local" || c.Authority != "synthetic_fixture" || !identifier.MatchString(c.ContractID) || c.ContractVersion < 1 || !identifier.MatchString(c.Owner) || !identifier.MatchString(input.SourceLabel) {
		fail(w, 422, "synthetic local contract and named fixture owner required")
		return
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(c.Schema, &document) != nil {
		fail(w, 422, "schema document must be an object")
		return
	}
	switch c.Dialect {
	case "https://json-schema.org/draft/2020-12/schema":
		if document["openapi"] != nil {
			fail(w, 422, "dialect mismatch")
			return
		}
	case "openapi-3.0", "openapi-3.1":
		var version string
		_ = json.Unmarshal(document["openapi"], &version)
		if !strings.HasPrefix(version, strings.TrimPrefix(c.Dialect, "openapi-")+".") {
			fail(w, 422, "dialect mismatch")
			return
		}
	default:
		fail(w, 422, "unsupported dialect")
		return
	}
	var result struct {
		Normalized struct {
			IRVersion   int             `json:"ir_version"`
			Root        json.RawMessage `json:"root"`
			Diagnostics []string        `json:"diagnostics"`
		} `json:"normalized"`
	}
	if err := r.engine.Call(req.Context(), map[string]any{"operation": "normalize", "document": c.Schema, "pointer": input.Pointer}, &result); err != nil {
		fail(w, 422, err.Error())
		return
	}
	// Diagnostics and source annotations are provenance, separate from the IR hash.
	normalized, _ := json.Marshal(result.Normalized)
	canonical, _ := json.Marshal(struct {
		IRVersion int             `json:"ir_version"`
		Root      json.RawMessage `json:"root"`
	}{result.Normalized.IRVersion, result.Normalized.Root})
	version := ContractVersion{Contract: c, Pointer: input.Pointer, SourceLabel: input.SourceLabel, SourceHash: digest(c.Schema), NormalizedHash: digest(canonical), Normalized: normalized, ImportedAt: r.now()}
	key := contractKey(c.ContractID, c.ContractVersion)
	if existing, ok := r.state.Contracts[key]; ok {
		left, _ := json.Marshal(existing.Contract)
		right, _ := json.Marshal(c)
		if string(left) != string(right) || existing.Pointer != input.Pointer || existing.SourceLabel != input.SourceLabel {
			fail(w, 409, "contract versions are immutable")
			return
		}
		respond(w, 200, existing)
		return
	}
	next := r.clone()
	next.Contracts[key] = version
	if r.commit(next) != nil {
		fail(w, 503, "registry persistence failed")
		return
	}
	respond(w, 201, version)
}

func (r *Registry) bind(w http.ResponseWriter, req *http.Request) {
	var input Binding
	if !readRequest(w, req, &input) {
		return
	}
	route, ok := r.routes[input.RouteID]
	version, found := r.state.Contracts[contractKey(input.ContractID, input.ContractVersion)]
	if !ok || !found || route.ContractID != input.ContractID || input.NormalizedHash != "" || !input.PublishedAt.IsZero() {
		fail(w, 422, "selected fixture route and imported contract version required")
		return
	}
	input.NormalizedHash = version.NormalizedHash
	input.PublishedAt = r.now()
	next := r.clone()
	next.Bindings[input.RouteID] = input
	if r.commit(next) != nil {
		fail(w, 503, "registry persistence failed")
		return
	}
	respond(w, 200, input)
}

func (r *Registry) drift(w http.ResponseWriter, req *http.Request) {
	var input struct {
		RouteID string          `json:"route_id"`
		Body    string          `json:"body"`
		Context json.RawMessage `json:"context"`
	}
	if !readRequest(w, req, &input) {
		return
	}
	route, exists := r.routes[input.RouteID]
	binding, bound := r.state.Bindings[input.RouteID]
	if !exists || !bound {
		fail(w, 422, "published fixture binding required")
		return
	}
	version := r.state.Contracts[contractKey(binding.ContractID, binding.ContractVersion)]
	var result struct {
		Detection Detection `json:"detection"`
	}
	if err := r.engine.Call(req.Context(), map[string]any{"operation": "detect", "document": version.Contract.Schema, "pointer": version.Pointer, "body": input.Body, "context": input.Context}, &result); err != nil {
		fail(w, 422, err.Error())
		return
	}
	d := result.Detection
	if d.Class == "healthy" {
		respond(w, 200, map[string]any{"detection": d, "incident": nil})
		return
	}
	partition := Partition{route.TenantID, route.EnvironmentID, route.RouteID, route.ProviderVersion, route.Direction, route.EventFamily, binding.ContractID, binding.ContractVersion, binding.NormalizedHash}
	fingerprint, _ := json.Marshal(struct {
		Partition     Partition
		Class, Reason string
		Changes       []Change
	}{partition, d.Class, d.Reason, d.Changes})
	id := "inc_" + strings.TrimPrefix(digest(fingerprint), "sha256:")
	now := r.now()
	incident, found := r.state.Incidents[id]
	if !found {
		incident = Incident{ID: id, Partition: partition, Class: d.Class, Reason: d.Reason, State: "provisional", Owner: version.Contract.Owner, FirstSeen: now, Changes: d.Changes}
	}
	incident.LastSeen = now
	incident.Observations++
	var quality struct {
		CaptureQuality string `json:"capture_quality"`
	}
	_ = json.Unmarshal(input.Context, &quality)
	incident.Evidence = append(incident.Evidence, Evidence{Reference: fmtEvidence(incident.Observations), SeenAt: now, CaptureQuality: quality.CaptureQuality, SampleProbability: d.SampleProbability})
	if len(incident.Evidence) > 32 {
		incident.Evidence = incident.Evidence[len(incident.Evidence)-32:]
	}
	qualifying := 0
	for _, evidence := range incident.Evidence {
		if evidence.CaptureQuality == "complete" && now.Sub(evidence.SeenAt) <= 5*time.Minute {
			qualifying++
		}
	}
	if (d.Class == "structural" && qualifying >= 3) || d.Reason == "explicit_invariant_violation" {
		incident.State = "confirmed"
	}
	next := r.clone()
	next.Incidents[id] = incident
	if r.commit(next) != nil {
		fail(w, 503, "registry persistence failed")
		return
	}
	respond(w, 200, map[string]any{"detection": d, "incident": incident})
}

func fmtEvidence(number int) string { return "observation:" + strconv.Itoa(number) }

func (r *Registry) incident(w http.ResponseWriter, req *http.Request) {
	incident, ok := r.state.Incidents[req.PathValue("id")]
	if !ok {
		fail(w, 404, "incident not found")
		return
	}
	respond(w, 200, incident)
}

func (r *Registry) review(w http.ResponseWriter, req *http.Request) {
	incident, ok := r.state.Incidents[req.PathValue("id")]
	if !ok {
		fail(w, 404, "incident not found")
		return
	}
	var input struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Reviewer string `json:"reviewer"`
	}
	if !readRequest(w, req, &input) {
		return
	}
	if incident.Class != "structural" || !identifier.MatchString(input.Reviewer) || input.From == input.To || !pointer(input.From) || !pointer(input.To) {
		fail(w, 422, "structural incident, valid distinct pointers and named reviewer required")
		return
	}
	incident.Review = &MappingReview{From: input.From, To: input.To, Reviewer: input.Reviewer, ReviewedAt: r.now(), AuthorizesRepair: false}
	next := r.clone()
	next.Incidents[incident.ID] = incident
	if r.commit(next) != nil {
		fail(w, 503, "registry persistence failed")
		return
	}
	respond(w, 200, incident)
}

func pointer(value string) bool {
	if len(value) > 1024 || !strings.HasPrefix(value, "/") {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] == '~' {
			i++
			if i == len(value) || (value[i] != '0' && value[i] != '1') {
				return false
			}
		}
	}
	return true
}

// Stable key cursors bound every list response. Tokens are local keys, not URLs.
func page(req *http.Request, keys []string) ([]string, string, error) {
	limit := 50
	if raw := req.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return nil, "", errors.New("limit must be 1..100")
		}
		limit = n
	}
	sort.Strings(keys)
	start := sort.SearchStrings(keys, req.URL.Query().Get("after"))
	if start < len(keys) && keys[start] == req.URL.Query().Get("after") {
		start++
	}
	end := min(start+limit, len(keys))
	cursor := ""
	if end < len(keys) {
		cursor = keys[end-1]
	}
	return keys[start:end], cursor, nil
}

func (r *Registry) listContracts(w http.ResponseWriter, req *http.Request) {
	keys := make([]string, 0, len(r.state.Contracts))
	for key := range r.state.Contracts {
		keys = append(keys, key)
	}
	keys, cursor, err := page(req, keys)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	items := make([]ContractVersion, 0, len(keys))
	for _, key := range keys {
		items = append(items, r.state.Contracts[key])
	}
	respond(w, 200, map[string]any{"items": items, "next": cursor})
}
func (r *Registry) listBindings(w http.ResponseWriter, req *http.Request) {
	keys := make([]string, 0, len(r.state.Bindings))
	for key := range r.state.Bindings {
		keys = append(keys, key)
	}
	keys, cursor, err := page(req, keys)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	items := make([]Binding, 0, len(keys))
	for _, key := range keys {
		items = append(items, r.state.Bindings[key])
	}
	respond(w, 200, map[string]any{"items": items, "next": cursor})
}
func (r *Registry) listIncidents(w http.ResponseWriter, req *http.Request) {
	keys := make([]string, 0, len(r.state.Incidents))
	for key, incident := range r.state.Incidents {
		if route := req.URL.Query().Get("route_id"); route == "" || route == incident.Partition.RouteID {
			keys = append(keys, key)
		}
	}
	keys, cursor, err := page(req, keys)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	items := make([]Incident, 0, len(keys))
	for _, key := range keys {
		items = append(items, r.state.Incidents[key])
	}
	respond(w, 200, map[string]any{"items": items, "next": cursor, "traffic_impact": "unknown; no request denominator collected"})
}
