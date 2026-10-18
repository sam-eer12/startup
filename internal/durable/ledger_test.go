package durable

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sam-eer12/startup/internal/trust"
	"github.com/stripe/stripe-go/v84/webhook"
)

func testConnection(t *testing.T, schema string) *pgx.Conn {
	t.Helper()
	url := os.Getenv("MESH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set MESH_TEST_DATABASE_URL for real PostgreSQL crash tests")
	}
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if schema != "" {
		config.RuntimeParams["search_path"] = schema
	}
	conn, err := pgx.ConnectConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}
func syntheticEvent(point Boundary) []byte {
	return []byte(fmt.Sprintf(`{"id":"evt_%s","type":"customer.updated","secret":"seed_LOCAL_ONLY_3421"}`, point))
}
func signature(raw []byte) string {
	return webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: raw, Secret: "whsec_synthetic_only", Timestamp: time.Now()}).Header
}
func acceptor(conn *pgx.Conn, objects *Objects) *Acceptor {
	return &Acceptor{Conn: conn, Objects: objects, Scope: trust.Scope{TenantID: "tenant_local", EnvironmentID: "env_local", RouteID: "route_stripe"}, Account: "acct_synthetic", StripeSecret: "whsec_synthetic_only"}
}

// This subprocess dies immediately at the injected boundary. Deferred rollback,
// file closes and Go cleanup do not run; PostgreSQL must recover the transaction.
func TestCrashProcess(t *testing.T) {
	if os.Getenv("MESH_CRASH_CHILD") != "1" {
		return
	}
	conn := testConnection(t, os.Getenv("MESH_CRASH_SCHEMA"))
	objects, err := NewObjects(os.Getenv("MESH_CRASH_OBJECTS"), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	point := Boundary(os.Getenv("MESH_CRASH_POINT"))
	a := acceptor(conn, objects)
	a.Crash = func(p Boundary) error {
		if p == point {
			os.Exit(86)
		}
		return nil
	}
	raw := syntheticEvent(point)
	if _, err := a.Accept(context.Background(), raw, signature(raw)); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash boundary not reached")
}

func TestPostgresCrashMatrix(t *testing.T) {
	ctx := context.Background()
	admin := testConnection(t, "")
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "spike_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	conn := testConnection(t, schema)
	if _, err := conn.Exec(ctx, Schema); err != nil {
		t.Fatal(err)
	}
	for _, point := range []Boundary{BeforeObject, BeforeCommit, AfterCommit, BeforeResponse} {
		t.Run(string(point), func(t *testing.T) {
			root := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashProcess$")
			cmd.Env = append(os.Environ(), "MESH_CRASH_CHILD=1", "MESH_CRASH_SCHEMA="+schema, "MESH_CRASH_OBJECTS="+root, "MESH_CRASH_POINT="+string(point))
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 86 {
				t.Fatalf("subprocess did not crash at %s: %v %s", point, err, output)
			}
			var count int
			raw := syntheticEvent(point)
			id := "evt_" + string(point)
			if err = conn.QueryRow(ctx, `SELECT count(*) FROM spike_events WHERE event_id=$1`, id).Scan(&count); err != nil {
				t.Fatal(err)
			}
			committed := point == AfterCommit || point == BeforeResponse
			if (count == 1) != committed {
				t.Fatalf("unexpected commit state at %s: %d", point, count)
			}
			objects, err := NewObjects(root, bytes.Repeat([]byte{4}, 32))
			if err != nil {
				t.Fatal(err)
			}
			if committed {
				body, err := objects.Get(trust.Digest(raw))
				if err != nil || !bytes.Equal(body, raw) {
					t.Fatal("accepted event lost its encrypted object", err)
				}
			}
			a := acceptor(conn, objects)
			receipt, err := a.Accept(ctx, raw, signature(raw))
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Duplicate != committed {
				t.Fatal("retry did not resolve original acceptance")
			}
			receipt, err = a.Accept(ctx, raw, signature(raw))
			if err != nil || !receipt.Duplicate {
				t.Fatal("second retry not deduplicated", err)
			}
			if err = conn.QueryRow(ctx, `SELECT count(*) FROM spike_jobs WHERE event_id=$1 AND state='pending'`, id).Scan(&count); err != nil || count != 1 {
				t.Fatal("event and job not atomic", err, count)
			}
			changed := append(bytes.Clone(raw), ' ')
			if _, err = a.Accept(ctx, changed, signature(changed)); !errors.Is(err, ErrConflict) {
				t.Fatal("conflicting duplicate accepted", err)
			}
			if _, err = a.Accept(ctx, raw, "t=0,v1=00"); err == nil {
				t.Fatal("unsigned event accepted")
			}
		})
	}
}
