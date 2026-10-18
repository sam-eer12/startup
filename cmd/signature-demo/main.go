// signature-demo writes disposable synthetic signature fixtures, never keys.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/sam-eer12/startup/internal/trust"
	"github.com/stripe/stripe-go/v84/webhook"
)

func run() error {
	out := flag.String("out", ".local/week2-signatures", "synthetic fixture directory")
	flag.Parse()
	original, err := os.ReadFile("fixtures/synthetic/stripe/field-move.json")
	if err != nil {
		return err
	}
	changed, err := os.ReadFile("fixtures/golden/stripe/field-move.json")
	if err != nil {
		return err
	}
	now := time.Now()
	secret := "whsec_public_synthetic_fixture_only"
	provider := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: original, Secret: secret, Timestamp: now})
	if err = trust.VerifyStripe(original, provider.Header, secret, now); err != nil {
		return err
	}
	if trust.VerifyStripe(changed, provider.Header, secret, now) == nil {
		return fmt.Errorf("mutation unexpectedly kept provider authentication")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	scope := trust.Scope{TenantID: "tenant_local", EnvironmentID: "env_local", RouteID: "route_stripe", Audience: "synthetic-receiver"}
	envelope := trust.Envelope{SchemaVersion: 1, TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RouteID: scope.RouteID, Audience: scope.Audience, EventID: "evt_synthetic_001", DeliveryID: "delivery_synthetic_001", AttemptID: "attempt_synthetic_001", Provider: "stripe", ProviderAccount: "acct_synthetic_only", ProviderType: "customer.updated", ProviderVersion: "fixture-v1", OriginalSHA256: trust.Digest(original), AdapterSHA256: trust.Digest([]byte("synthetic-guarded-move-v1")), ContractSHA256: trust.Digest([]byte("synthetic-consumer-contract-v1")), VerifiedAt: now.Unix(), IssuedAt: now.Unix(), Data: changed}
	body, signature, err := trust.Sign(envelope, private, "disposable_local_key")
	if err != nil {
		return err
	}
	if _, err = trust.Verify(body, signature, public, "disposable_local_key", scope, now); err != nil {
		return err
	}
	jwks, err := json.MarshalIndent(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: public, KeyID: "disposable_local_key", Algorithm: string(jose.EdDSA), Use: "sig"}}}, "", "  ")
	if err != nil {
		return err
	}
	report, err := json.MarshalIndent(map[string]any{"profile": "synthetic-only", "generated_at": now.UTC().Format(time.RFC3339), "provider_sdk": "stripe-go/v84 v84.4.1", "original_verified": true, "changed_body_provider_signature_rejected": true, "mesh_detached_signature_verified": true, "public_fixture_webhook_secret": secret, "key_policy": "fresh disposable Ed25519 key; private key never persisted", "adapter_and_contract_hashes": "synthetic identity markers, not release artifacts", "timestamp_tolerance_seconds": 300}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(*out, 0700); err != nil {
		return err
	}
	for name, value := range map[string][]byte{"original.json": original, "stripe-signature.txt": []byte(provider.Header + "\n"), "mesh-envelope.json": body, "mesh-signature.txt": []byte(signature + "\n"), "jwks.json": jwks, "report.json": report} {
		if err = os.WriteFile(filepath.Join(*out, name), value, 0600); err != nil {
			return err
		}
	}
	fmt.Println("Synthetic original/mutated/mesh trust demo passed; fixtures in", *out)
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
