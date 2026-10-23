package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/sam-eer12/startup/connectors"
)

const maxStateBytes = 8 * 1024 * 1024

type Contract struct {
	SchemaVersion   int             `json:"schema_version"`
	ContractID      string          `json:"contract_id"`
	ContractVersion int             `json:"contract_version"`
	TenantID        string          `json:"tenant_id"`
	EnvironmentID   string          `json:"environment_id"`
	Authority       string          `json:"authority"`
	Owner           string          `json:"owner"`
	Dialect         string          `json:"dialect"`
	Schema          json.RawMessage `json:"schema"`
}

type ContractVersion struct {
	Contract       Contract        `json:"contract"`
	Pointer        string          `json:"pointer"`
	SourceLabel    string          `json:"source_label"`
	SourceHash     string          `json:"source_hash"`
	NormalizedHash string          `json:"normalized_hash"`
	Normalized     json.RawMessage `json:"normalized"`
	ImportedAt     time.Time       `json:"imported_at"`
}

type Binding struct {
	RouteID         string    `json:"route_id"`
	ContractID      string    `json:"contract_id"`
	ContractVersion int       `json:"contract_version"`
	NormalizedHash  string    `json:"normalized_hash"`
	PublishedAt     time.Time `json:"published_at"`
}

type Change struct {
	Code string `json:"code"`
	Path string `json:"path"`
}
type Detection struct {
	Class                   string          `json:"class"`
	Reason                  string          `json:"reason"`
	Changes                 []Change        `json:"changes"`
	Shape                   json.RawMessage `json:"shape"`
	AutomaticRepairEligible bool            `json:"automatic_repair_eligible"`
	Confidence              string          `json:"confidence"`
	SampleProbability       float64         `json:"sample_probability"`
	SemanticVerification    string          `json:"semantic_verification"`
}

type Partition struct {
	TenantID        string `json:"tenant_id"`
	EnvironmentID   string `json:"environment_id"`
	RouteID         string `json:"route_id"`
	ProviderVersion string `json:"provider_version"`
	Direction       string `json:"direction"`
	EventFamily     string `json:"event_family"`
	ContractID      string `json:"contract_id"`
	ContractVersion int    `json:"contract_version"`
	NormalizedHash  string `json:"normalized_hash"`
}

type Evidence struct {
	Reference         string    `json:"reference"`
	SeenAt            time.Time `json:"seen_at"`
	CaptureQuality    string    `json:"capture_quality"`
	SampleProbability float64   `json:"sample_probability"`
}

type MappingReview struct {
	From             string    `json:"from"`
	To               string    `json:"to"`
	Reviewer         string    `json:"reviewer"`
	ReviewedAt       time.Time `json:"reviewed_at"`
	AuthorizesRepair bool      `json:"authorizes_repair"`
}

type Incident struct {
	ID           string         `json:"id"`
	Partition    Partition      `json:"partition"`
	Class        string         `json:"class"`
	Reason       string         `json:"reason"`
	State        string         `json:"state"`
	Owner        string         `json:"owner"`
	FirstSeen    time.Time      `json:"first_seen"`
	LastSeen     time.Time      `json:"last_seen"`
	Observations int            `json:"observations"`
	Changes      []Change       `json:"changes"`
	Evidence     []Evidence     `json:"evidence"`
	Review       *MappingReview `json:"mapping_review,omitempty"`
}

type state struct {
	SchemaVersion int                        `json:"schema_version"`
	Contracts     map[string]ContractVersion `json:"contracts"`
	Bindings      map[string]Binding         `json:"bindings"`
	Incidents     map[string]Incident        `json:"incidents"`
}

type Registry struct {
	mu     sync.Mutex
	root   string
	engine Engine
	routes map[string]connectors.Route
	state  state
	now    func() time.Time
	lock   *os.File
}

// Open owns a private single-process lab directory. Atomic replacement prevents
// partial JSON commits; this is not a PostgreSQL production registry.
func Open(root string, engine Engine) (*Registry, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	routes, err := connectors.Routes()
	if err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(root, ".writer.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("registry already has a writer")
	}
	success := false
	defer func() {
		if !success {
			lock.Close()
		}
	}()
	r := &Registry{root: root, engine: engine, routes: routes, lock: lock, now: func() time.Time { return time.Now().UTC() }}
	r.state = state{SchemaVersion: 1, Contracts: map[string]ContractVersion{}, Bindings: map[string]Binding{}, Incidents: map[string]Incident{}}
	file, err := os.Open(filepath.Join(root, "registry.json"))
	if errors.Is(err, os.ErrNotExist) {
		success = true
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > maxStateBytes {
		return nil, errors.New("registry file limit")
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&r.state); err != nil {
		return nil, errors.New("invalid registry file")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing registry content")
	}
	if r.state.SchemaVersion != 1 || r.state.Contracts == nil || r.state.Bindings == nil || r.state.Incidents == nil {
		return nil, errors.New("unsupported registry state")
	}
	if len(r.state.Contracts) > 256 || len(r.state.Incidents) > 1024 {
		return nil, errors.New("registry record quota")
	}
	for _, binding := range r.state.Bindings {
		version, ok := r.state.Contracts[contractKey(binding.ContractID, binding.ContractVersion)]
		if !ok || version.NormalizedHash != binding.NormalizedHash {
			return nil, errors.New("invalid persisted binding")
		}
	}
	success = true
	return r, nil
}

func (r *Registry) Close() error { return r.lock.Close() }

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func contractKey(id string, version int) string { return fmt.Sprintf("%s:%d", id, version) }

func (r *Registry) clone() state {
	body, _ := json.Marshal(r.state)
	var next state
	_ = json.Unmarshal(body, &next)
	return next
}

func (r *Registry) commit(next state) error {
	if len(next.Contracts) > 256 || len(next.Incidents) > 1024 {
		return errors.New("local registry record quota exhausted")
	}
	body, err := json.Marshal(next)
	if err != nil || len(body) > maxStateBytes {
		return errors.New("local registry byte quota exhausted")
	}
	file, err := os.CreateTemp(r.root, ".registry-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(r.root, "registry.json")); err != nil {
		return err
	}
	// Once rename succeeds the new state is visible even if directory sync fails.
	r.state = next
	directory, err := os.Open(r.root)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
