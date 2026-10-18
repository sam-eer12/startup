package durable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sam-eer12/startup/internal/capture"
	"github.com/sam-eer12/startup/internal/trust"
)

const Schema = `
CREATE TABLE IF NOT EXISTS spike_events (
 tenant_id text NOT NULL, environment_id text NOT NULL, route_id text NOT NULL,
 provider_account text NOT NULL, event_id text NOT NULL, body_sha256 text NOT NULL,
 accepted_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,environment_id,route_id,provider_account,event_id)
);
CREATE TABLE IF NOT EXISTS spike_jobs (
 tenant_id text NOT NULL, environment_id text NOT NULL, route_id text NOT NULL,
 provider_account text NOT NULL, event_id text NOT NULL, state text NOT NULL DEFAULT 'pending',
 PRIMARY KEY (tenant_id,environment_id,route_id,provider_account,event_id),
 FOREIGN KEY (tenant_id,environment_id,route_id,provider_account,event_id)
 REFERENCES spike_events(tenant_id,environment_id,route_id,provider_account,event_id)
);`

type Boundary string

const (
	BeforeObject   Boundary = "before_object"
	BeforeCommit   Boundary = "before_commit"
	AfterCommit    Boundary = "after_commit"
	BeforeResponse Boundary = "before_response"
)

var ErrConflict = errors.New("same event identity has different original bytes")

// Acceptor's scope/account/secret come from trusted configuration, never headers
// or fields supplied by the caller. One object directory belongs to this scope.
type Acceptor struct {
	Conn         *pgx.Conn
	Objects      *Objects
	Scope        trust.Scope
	Account      string
	StripeSecret string
	Now          func() time.Time
	// Crash is fault injection for local tests, never a remotely supplied option.
	Crash func(Boundary) error
}
type Receipt struct {
	EventID, ObjectRef string
	Duplicate          bool
}

func (a *Acceptor) boundary(point Boundary) error {
	if a.Crash != nil {
		return a.Crash(point)
	}
	return nil
}

// Accept success means object fsync and event+pending-job transaction commit.
// It makes no claim about downstream business processing or broker publication.
func (a *Acceptor) Accept(ctx context.Context, raw []byte, signature string) (Receipt, error) {
	if a.Conn == nil || a.Objects == nil || a.Scope.TenantID == "" || a.Scope.EnvironmentID == "" || a.Scope.RouteID == "" || a.Account == "" {
		return Receipt{}, errors.New("trusted configuration required")
	}
	now := time.Now()
	if a.Now != nil {
		now = a.Now()
	}
	if err := trust.VerifyStripe(raw, signature, a.StripeSecret, now); err != nil {
		return Receipt{}, err
	}
	if _, err := capture.Decode(raw); err != nil {
		return Receipt{}, err
	}
	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &event); err != nil || event.ID == "" || len(event.ID) > 255 || event.Type != "customer.updated" {
		return Receipt{}, errors.New("unsupported synthetic event")
	}
	if err := a.boundary(BeforeObject); err != nil {
		return Receipt{}, err
	}
	ref, err := a.Objects.Put(raw)
	if err != nil {
		return Receipt{}, err
	}
	tx, err := a.Conn.Begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SET LOCAL synchronous_commit = on"); err != nil {
		return Receipt{}, err
	}
	args := []any{a.Scope.TenantID, a.Scope.EnvironmentID, a.Scope.RouteID, a.Account, event.ID}
	inserted, err := tx.Exec(ctx, `INSERT INTO spike_events (tenant_id,environment_id,route_id,provider_account,event_id,body_sha256) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, append(args, ref)...)
	if err != nil {
		return Receipt{}, err
	}
	var existing string
	if err = tx.QueryRow(ctx, `SELECT body_sha256 FROM spike_events WHERE tenant_id=$1 AND environment_id=$2 AND route_id=$3 AND provider_account=$4 AND event_id=$5`, args...).Scan(&existing); err != nil {
		return Receipt{}, err
	}
	if existing != ref {
		return Receipt{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO spike_jobs (tenant_id,environment_id,route_id,provider_account,event_id) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, args...); err != nil {
		return Receipt{}, err
	}
	if err = a.boundary(BeforeCommit); err != nil {
		return Receipt{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Receipt{}, fmt.Errorf("acceptance commit uncertain: %w", err)
	}
	if err = a.boundary(AfterCommit); err != nil {
		return Receipt{}, err
	}
	if err = a.boundary(BeforeResponse); err != nil {
		return Receipt{}, err
	}
	return Receipt{EventID: event.ID, ObjectRef: ref, Duplicate: inserted.RowsAffected() == 0}, nil
}
