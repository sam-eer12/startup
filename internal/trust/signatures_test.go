package trust

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v84/webhook"
)

func TestOriginalProviderBytesAndClock(t *testing.T) {
	now := time.Unix(1800000000, 0)
	raw := []byte(`{"id":"evt_synthetic","type":"customer.updated","data":{"value":"<>&"}}`)
	header := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: raw, Secret: "whsec_synthetic_only", Timestamp: now}).Header
	if err := VerifyStripe(raw, header, "whsec_synthetic_only", now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body           []byte
		header, secret string
		now            time.Time
	}{
		{append(bytes.Clone(raw), ' '), header, "whsec_synthetic_only", now},
		{raw, header, "wrong", now},
		{raw, header, "whsec_synthetic_only", now.Add(6 * time.Minute)},
		{raw, header, "whsec_synthetic_only", now.Add(-6 * time.Minute)},
		{raw, header + ",t=1800000000", "whsec_synthetic_only", now},
		{raw, "v1=00", "whsec_synthetic_only", now},
	} {
		if VerifyStripe(tc.body, tc.header, tc.secret, tc.now) == nil {
			t.Fatal("accepted forged, stale or ambiguous original")
		}
	}
	mac := hmac.New(sha256.New, []byte("shopify_synthetic"))
	mac.Write(raw)
	shopify := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if VerifyShopify(raw, shopify, "shopify_synthetic") != nil {
		t.Fatal("Shopify fixture failed")
	}
	if VerifyShopify(append(bytes.Clone(raw), ' '), shopify, "shopify_synthetic") == nil {
		t.Fatal("mutated Shopify bytes accepted")
	}
}

func TestDetachedMeshTrustAndStableData(t *testing.T) {
	now := time.Unix(1800000000, 0)
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32))
	pub := key.Public().(ed25519.PublicKey)
	scope := Scope{"tenant_local", "env_local", "route_stripe", "synthetic-receiver"}
	envelope := Envelope{SchemaVersion: 1, TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RouteID: scope.RouteID, Audience: scope.Audience, EventID: "evt_1", DeliveryID: "delivery_1", AttemptID: "attempt_1", Provider: "stripe", ProviderAccount: "acct_synthetic", ProviderType: "customer.updated", ProviderVersion: "fixture-v1", OriginalSHA256: Digest([]byte("original")), AdapterSHA256: Digest([]byte("adapter")), ContractSHA256: Digest([]byte("contract")), VerifiedAt: now.Unix(), IssuedAt: now.Unix(), Data: json.RawMessage(`{"id":9007199254740993,"label":"<>&","note":null}`)}
	body, signature, err := Sign(envelope, key, "key_fixture")
	if err != nil {
		t.Fatal(err)
	}
	first, err := Verify(body, signature, pub, "key_fixture", scope, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body  []byte
		sig   string
		key   ed25519.PublicKey
		kid   string
		scope Scope
		now   time.Time
	}{
		{append(bytes.Clone(body), ' '), signature, pub, "key_fixture", scope, now},
		{body, signature, pub, "unknown", scope, now},
		{body, signature, ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32)).Public().(ed25519.PublicKey), "key_fixture", scope, now},
		{body, signature, pub, "key_fixture", Scope{"other", "env_local", "route_stripe", "synthetic-receiver"}, now},
		{body, signature, pub, "key_fixture", Scope{"tenant_local", "env_local", "route_stripe", "other"}, now},
		{body, signature, pub, "key_fixture", scope, now.Add(6 * time.Minute)},
	} {
		if _, err := Verify(tc.body, tc.sig, tc.key, tc.kid, tc.scope, tc.now); err == nil {
			t.Fatal("accepted invalid mesh trust")
		}
	}
	envelope.AttemptID = "attempt_2"
	envelope.IssuedAt = now.Add(time.Minute).Unix()
	retry, retrySignature, err := Sign(envelope, key, "key_fixture")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Verify(retry, retrySignature, pub, "key_fixture", scope, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.DataSHA256 != second.DataSHA256 || !bytes.Equal(first.Data, second.Data) || first.DeliveryID != second.DeliveryID {
		t.Fatal("retry changed frozen logical data")
	}
}
