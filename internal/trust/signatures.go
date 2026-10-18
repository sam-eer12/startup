// Package trust contains the synthetic Week 2 signature boundary spike.
package trust

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/sam-eer12/startup/internal/capture"
	"github.com/stripe/stripe-go/v84/webhook"
)

const MediaType = "application/vnd.mesh.delivery.v1+json"
const Tolerance = 5 * time.Minute

var ErrTrust = errors.New("signature or trust policy rejected")

// VerifyStripe authenticates the original bytes with Stripe's SDK, then applies
// a symmetric clock window and rejects ambiguous/missing timestamps. No parsing
// or transformation may precede this call at the ingress boundary.
func VerifyStripe(raw []byte, header, secret string, now time.Time) error {
	if len(raw) > capture.MaxBodyBytes || len(header) > 8192 || secret == "" {
		return ErrTrust
	}
	var timestamp int64
	count := 0
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return ErrTrust
		}
		if key == "t" {
			var err error
			timestamp, err = strconv.ParseInt(value, 10, 64)
			if err != nil || timestamp < 0 {
				return ErrTrust
			}
			count++
		}
	}
	if count != 1 {
		return ErrTrust
	}
	signed := time.Unix(timestamp, 0)
	if signed.Before(now.Add(-Tolerance)) || signed.After(now.Add(Tolerance)) {
		return ErrTrust
	}
	if webhook.ValidatePayloadIgnoringTolerance(raw, header, secret) != nil {
		return ErrTrust
	}
	return nil
}

// Shopify authenticates the raw body using base64 HMAC-SHA256, without Stripe's
// timestamp prefix. Freshness/dedupe needs the provider's separate delivery ID.
func VerifyShopify(raw []byte, header, secret string) error {
	if len(raw) > capture.MaxBodyBytes || len(header) > 128 || secret == "" {
		return ErrTrust
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(header)
	if err != nil {
		return ErrTrust
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return ErrTrust
	}
	return nil
}

func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type Scope struct{ TenantID, EnvironmentID, RouteID, Audience string }
type Envelope struct {
	SchemaVersion   int             `json:"schema_version"`
	TenantID        string          `json:"tenant_id"`
	EnvironmentID   string          `json:"environment_id"`
	RouteID         string          `json:"route_id"`
	EventID         string          `json:"event_id"`
	DeliveryID      string          `json:"delivery_id"`
	Provider        string          `json:"provider"`
	ProviderAccount string          `json:"provider_account"`
	ProviderType    string          `json:"provider_type"`
	ProviderVersion string          `json:"provider_version"`
	OriginalSHA256  string          `json:"original_body_sha256"`
	AdapterSHA256   string          `json:"adapter_sha256"`
	ContractSHA256  string          `json:"target_contract_sha256"`
	VerifiedAt      int64           `json:"verified_at"`
	Data            json.RawMessage `json:"data"`
	DataSHA256      string          `json:"data_sha256"`
	IssuedAt        int64           `json:"issued_at"`
	AttemptID       string          `json:"attempt_id"`
	Audience        string          `json:"audience"`
}

// FreezeData runs once before first delivery. Persist these exact compact bytes
// and the pinned artifact; a retry must not regenerate them under a new adapter.
func FreezeData(data []byte) (json.RawMessage, error) {
	if _, err := capture.Decode(data); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Compact(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func Sign(envelope Envelope, key ed25519.PrivateKey, kid string) ([]byte, string, error) {
	if len(key) != ed25519.PrivateKeySize || kid == "" {
		return nil, "", ErrTrust
	}
	data, err := FreezeData(envelope.Data)
	if err != nil {
		return nil, "", err
	}
	envelope.Data, envelope.DataSHA256 = data, Digest(data)
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(envelope); err != nil {
		return nil, "", err
	}
	if body.Len() > capture.MaxBodyBytes {
		return nil, "", ErrTrust
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: key}, (&jose.SignerOptions{}).WithType(MediaType).WithHeader("kid", kid))
	if err != nil {
		return nil, "", err
	}
	signed, err := signer.Sign(body.Bytes())
	if err != nil {
		return nil, "", err
	}
	signature, err := signed.DetachedCompactSerialize()
	return body.Bytes(), signature, err
}

// Verify trusts only the configured key/scope, and authenticates exact envelope
// bytes before JSON parsing. It never fetches a key URL supplied by the sender.
func Verify(body []byte, signature string, key ed25519.PublicKey, kid string, scope Scope, now time.Time) (Envelope, error) {
	var envelope Envelope
	if kid == "" || scope.TenantID == "" || scope.EnvironmentID == "" || scope.RouteID == "" || scope.Audience == "" {
		return envelope, ErrTrust
	}
	if len(body) > capture.MaxBodyBytes || len(signature) > 8192 || len(key) != ed25519.PublicKeySize {
		return envelope, ErrTrust
	}
	parts := strings.Split(signature, ".")
	if len(parts) != 3 || parts[1] != "" {
		return envelope, ErrTrust
	}
	signed, err := jose.ParseDetached(signature, body, []jose.SignatureAlgorithm{jose.EdDSA})
	if err != nil || len(signed.Signatures) != 1 {
		return envelope, ErrTrust
	}
	header := signed.Signatures[0].Protected
	if header.KeyID != kid || len(header.ExtraHeaders) != 1 || header.ExtraHeaders["typ"] != MediaType || header.JSONWebKey != nil || signed.DetachedVerify(body, key) != nil {
		return envelope, ErrTrust
	}
	// The envelope and data both use the bounded, duplicate-rejecting parser.
	if _, err := capture.Decode(body); err != nil {
		return envelope, ErrTrust
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, ErrTrust
	}
	if envelope.SchemaVersion != 1 || envelope.TenantID != scope.TenantID || envelope.EnvironmentID != scope.EnvironmentID || envelope.RouteID != scope.RouteID || envelope.Audience != scope.Audience {
		return Envelope{}, ErrTrust
	}
	if envelope.EventID == "" || envelope.DeliveryID == "" || envelope.AttemptID == "" || envelope.Provider == "" || envelope.ProviderAccount == "" || envelope.ProviderType == "" || envelope.ProviderVersion == "" {
		return Envelope{}, ErrTrust
	}
	for _, digest := range []string{envelope.OriginalSHA256, envelope.AdapterSHA256, envelope.ContractSHA256, envelope.DataSHA256} {
		if len(digest) != 71 || !strings.HasPrefix(digest, "sha256:") {
			return Envelope{}, ErrTrust
		}
		if _, err := hex.DecodeString(digest[7:]); err != nil {
			return Envelope{}, ErrTrust
		}
	}
	if envelope.IssuedAt < now.Add(-Tolerance).Unix() || envelope.IssuedAt > now.Add(Tolerance).Unix() || envelope.VerifiedAt <= 0 || envelope.VerifiedAt > envelope.IssuedAt {
		return Envelope{}, ErrTrust
	}
	if _, err := capture.Decode(envelope.Data); err != nil || Digest(envelope.Data) != envelope.DataSHA256 {
		return Envelope{}, ErrTrust
	}
	return envelope, nil
}
