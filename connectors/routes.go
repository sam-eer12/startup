// Package connectors embeds reviewed local fixture routes. These are synthetic
// technical-spike inputs, not partner contracts or live-qualified connectors.
package connectors

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed */route.json
var files embed.FS

type Route struct {
	SchemaVersion   int           `json:"schema_version"`
	RouteID         string        `json:"route_id"`
	TenantID        string        `json:"tenant_id"`
	EnvironmentID   string        `json:"environment_id"`
	Connector       string        `json:"connector"`
	ProviderVersion string        `json:"provider_version"`
	EventFamily     string        `json:"event_family"`
	Direction       string        `json:"direction"`
	Mode            string        `json:"mode"`
	Scope           string        `json:"scope"`
	Owner           string        `json:"owner"`
	ContractID      string        `json:"contract_id"`
	ContractVersion int           `json:"contract_version"`
	PayloadPolicy   PayloadPolicy `json:"payload_policy"`
}

type PayloadPolicy struct {
	Locality                string `json:"locality"`
	TelemetryRetentionHours int    `json:"telemetry_retention_hours"`
	DurableAcceptance       bool   `json:"durable_acceptance"`
	SignatureTrust          string `json:"signature_trust"`
	RedactionPolicy         string `json:"redaction_policy"`
}

func Routes() (map[string]Route, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	routes := map[string]Route{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		body, err := files.ReadFile(entry.Name() + "/route.json")
		if err != nil {
			return nil, err
		}
		var route Route
		if err := json.Unmarshal(body, &route); err != nil {
			return nil, err
		}
		if route.SchemaVersion != 1 || route.Mode != "observe" || route.Scope != "synthetic_local" || route.PayloadPolicy.DurableAcceptance || route.PayloadPolicy.RedactionPolicy != "all-scalars-and-keys-v1" || route.PayloadPolicy.SignatureTrust != "unverified_fixture" {
			return nil, fmt.Errorf("route is not an eligible local fixture")
		}
		if _, exists := routes[route.RouteID]; exists {
			return nil, fmt.Errorf("duplicate route ID")
		}
		routes[route.RouteID] = route
	}
	return routes, nil
}
