package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testRegistry(t *testing.T) (*Registry, string, Engine) {
	t.Helper()
	path, err := filepath.Abs("../../target/debug/contract-ir")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("build Rust engine first: make contract-tool")
	}
	dir := t.TempDir()
	engine := Engine{Path: path}
	r, err := Open(dir, engine)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, dir, engine
}

func request(t *testing.T, r *Registry, method, path string, body any, want int) []byte {
	t.Helper()
	bytes, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(string(bytes)))
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, req)
	if w.Code != want {
		t.Fatalf("%s %s: status %d wanted %d: %s", method, path, w.Code, want, w.Body.String())
	}
	return w.Body.Bytes()
}

func importFixture(t *testing.T, r *Registry, family string, version int) ContractVersion {
	t.Helper()
	body, err := os.ReadFile("../../connectors/" + family + "/consumer-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract Contract
	if err := json.Unmarshal(body, &contract); err != nil {
		t.Fatal(err)
	}
	contract.ContractVersion = version
	response := request(t, r, "POST", "/v1/contracts", map[string]any{"contract": contract, "pointer": "", "source_label": "reviewed-fixture"}, 201)
	var result ContractVersion
	if json.Unmarshal(response, &result) != nil {
		t.Fatal("invalid imported version")
	}
	request(t, r, "POST", "/v1/bindings", map[string]any{"route_id": "route_" + family, "contract_id": contract.ContractID, "contract_version": version}, 200)
	return result
}

func driftFixture(t *testing.T, r *Registry, family, body, quality string) (Detection, *Incident) {
	t.Helper()
	response := request(t, r, "POST", "/v1/drift", map[string]any{"route_id": "route_" + family, "body": body, "context": map[string]any{"http_status": 200, "content_type": "application/json", "content_encoding": "identity", "capture_quality": quality, "sample_probability": 0.1, "business_decline": false, "invariant_violation": false}}, 200)
	var result struct {
		Detection Detection `json:"detection"`
		Incident  *Incident `json:"incident"`
	}
	if json.Unmarshal(response, &result) != nil {
		t.Fatal("invalid drift response")
	}
	return result.Detection, result.Incident
}

func TestRegistryPersistenceBindingsConfirmationAndReview(t *testing.T) {
	r, dir, engine := testRegistry(t)
	version := importFixture(t, r, "stripe", 1)
	if version.NormalizedHash == "" || version.SourceHash == "" {
		t.Fatal("provenance absent")
	}
	fixture, _ := os.ReadFile("../../fixtures/synthetic/stripe/field-move.json")
	start := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return start }
	var incident *Incident
	for i := 0; i < 3; i++ {
		d, got := driftFixture(t, r, "stripe", string(fixture), "complete")
		if d.Class != "structural" || d.AutomaticRepairEligible {
			t.Fatal("invalid diagnostic authority")
		}
		incident = got
		want := "provisional"
		if i == 2 {
			want = "confirmed"
		}
		if incident.State != want {
			t.Fatalf("state %s wanted %s", incident.State, want)
		}
	}
	request(t, r, "POST", "/v1/incidents/"+incident.ID+"/mapping-review", map[string]any{"from": "/data/object/metadata/presentation/label", "to": "/data/object/metadata/display_label", "reviewer": "fixture-reviewer"}, 200)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, engine)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	body := request(t, reopened, "GET", "/v1/incidents/"+incident.ID, nil, 200)
	var restored Incident
	_ = json.Unmarshal(body, &restored)
	if restored.State != "confirmed" || restored.Observations != 3 || restored.Review == nil || restored.Review.AuthorizesRepair || len(restored.Evidence) != 3 {
		t.Fatal("state lost or review promoted authority")
	}
	importFixture(t, reopened, "stripe", 2)
	_, newIncident := driftFixture(t, reopened, "stripe", string(fixture), "complete")
	if newIncident.ID == incident.ID || newIncident.State != "provisional" || newIncident.Partition.ContractVersion != 2 {
		t.Fatal("contract cohorts merged")
	}
}

func TestWindowQualityAndRedaction(t *testing.T) {
	r, dir, _ := testRegistry(t)
	importFixture(t, r, "generic-json", 1)
	start := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return start }
	for i := 0; i < 3; i++ {
		_, incident := driftFixture(t, r, "generic-json", `{"KEY_SECRET":"VALUE_SECRET"}`, "complete")
		if incident.State != "provisional" {
			t.Fatal("widely spaced observations confirmed")
		}
		start = start.Add(6 * time.Minute)
	}
	for i := 0; i < 3; i++ {
		d, incident := driftFixture(t, r, "generic-json", "{", "truncated")
		if d.Class != "unsupported" || incident.State != "provisional" {
			t.Fatal("incomplete evidence confirmed structural drift")
		}
	}
	persisted, err := os.ReadFile(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"KEY_SECRET", "VALUE_SECRET", "synthetic@example.invalid"} {
		if strings.Contains(string(persisted), secret) {
			t.Fatal("raw observation leaked into registry")
		}
	}
	baseline, _ := os.ReadFile("../../fixtures/synthetic/generic-json/baseline.json")
	d, incident := driftFixture(t, r, "generic-json", string(baseline), "complete")
	if d.Class != "healthy" || incident != nil || d.SemanticVerification != "unavailable_without_explicit_invariants" {
		t.Fatal("healthy schema claimed semantic verification")
	}
}

func TestImportImmutableAndFailClosed(t *testing.T) {
	r, _, _ := testRegistry(t)
	version := importFixture(t, r, "stripe", 1)
	input := map[string]any{"contract": version.Contract, "pointer": "", "source_label": "reviewed-fixture"}
	request(t, r, "POST", "/v1/contracts", input, 200)
	contract := version.Contract
	contract.Owner = "different-owner"
	input["contract"] = contract
	request(t, r, "POST", "/v1/contracts", input, 409)
	contract.ContractVersion = 2
	contract.Authority = "owner_approved"
	input["contract"] = contract
	request(t, r, "POST", "/v1/contracts", input, 422)
	contract.Authority = "synthetic_fixture"
	contract.Schema = json.RawMessage(`{"$ref":"http://127.0.0.1/private"}`)
	input["contract"] = contract
	request(t, r, "POST", "/v1/contracts", input, 422)
	request(t, r, "POST", "/v1/bindings", map[string]any{"route_id": "route_stripe", "contract_id": "contract_stripe", "contract_version": 9}, 422)
	request(t, r, "POST", "/v1/drift", map[string]any{"route_id": "route_shopify", "body": "{}", "context": map[string]any{}}, 422)
	request(t, r, "GET", "/v1/incidents?limit=101", nil, 400)
	req := httptest.NewRequest("POST", "/v1/contracts", strings.NewReader(`{"contract":{},"contract":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal("duplicate request accepted")
	}
	req = httptest.NewRequest("POST", "/v1/contracts", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://example.invalid")
	w = httptest.NewRecorder()
	r.Handler().ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("browser origin permitted")
	}
}

func TestWriterQuotaAndFailedCommitDoNotAcknowledge(t *testing.T) {
	r, dir, engine := testRegistry(t)
	if other, err := Open(dir, engine); err == nil {
		other.Close()
		t.Fatal("second writer allowed")
	}
	importFixture(t, r, "stripe", 1)
	old := r.root
	r.root = filepath.Join(dir, "missing-parent")
	request(t, r, "POST", "/v1/bindings", map[string]any{"route_id": "route_stripe", "contract_id": "contract_stripe", "contract_version": 1}, 503)
	r.root = old
	if r.state.Bindings["route_stripe"].ContractVersion != 1 {
		t.Fatal("state damaged")
	}
	r.mu.Lock()
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/incidents", nil))
	r.mu.Unlock()
	if w.Code != 503 {
		t.Fatal("unbounded request wait")
	}
}

func TestNormalizedHashExcludesAnnotationsAndImportPreservesLargeNumbers(t *testing.T) {
	r, _, _ := testRegistry(t)
	contract := Contract{SchemaVersion: 1, ContractID: "contract_stripe", ContractVersion: 1, TenantID: "tenant_local", EnvironmentID: "env_local", Authority: "synthetic_fixture", Owner: "fixture-owner", Dialect: "https://json-schema.org/draft/2020-12/schema", Schema: json.RawMessage(`{"type":"string","title":"first"}`)}
	importVersion := func(c Contract) ContractVersion {
		t.Helper()
		body := request(t, r, "POST", "/v1/contracts", map[string]any{"contract": c, "pointer": "", "source_label": "exact-fixture"}, 201)
		var result ContractVersion
		if json.Unmarshal(body, &result) != nil {
			t.Fatal("invalid version")
		}
		return result
	}
	first := importVersion(contract)
	contract.ContractVersion = 2
	contract.Schema = json.RawMessage(`{"title":"second","type":"string"}`)
	second := importVersion(contract)
	if first.NormalizedHash != second.NormalizedHash || first.SourceHash == second.SourceHash {
		t.Fatal("source provenance and normalized semantics conflated")
	}
	contract.ContractVersion = 3
	// This valid bounded token exceeds f64's range after exponent expansion.
	// Source metadata inspection must not convert nested schema numbers to f64.
	contract.Schema = json.RawMessage(`{"minimum":` + strings.Repeat("9", 100) + `e250}`)
	third := importVersion(contract)
	if third.NormalizedHash == "" {
		t.Fatal("exact large numeric constraint lost")
	}
}
