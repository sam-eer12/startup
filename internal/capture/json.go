// Package capture implements bounded, local-only observation. Observations are
// never authoritative consumer contracts and cannot authorize a repair.
package capture

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const (
	MaxBodyBytes = 256 * 1024
	MaxDepth     = 64
	MaxNodes     = 10000
)

var ErrInvalidJSON = errors.New("body is not eligible complete JSON")

// Decode rejects duplicate keys, invalid UTF-8, trailing documents and excessive
// resource use. UseNumber preserves integers above 2^53 and exact decimal tokens.
func Decode(body []byte) (any, error) {
	if len(body) > MaxBodyBytes || !utf8.Valid(body) {
		return nil, ErrInvalidJSON
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	nodes := 0
	value, err := readValue(d, 0, &nodes)
	if err != nil {
		return nil, ErrInvalidJSON
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrInvalidJSON
	}
	return value, nil
}

func readValue(d *json.Decoder, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > MaxDepth || *nodes > MaxNodes {
		return nil, ErrInvalidJSON
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		obj := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, ErrInvalidJSON
			}
			if _, duplicate := obj[name]; duplicate {
				return nil, ErrInvalidJSON
			}
			value, err := readValue(d, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			obj[name] = value
		}
		if end, err := d.Token(); err != nil || end != json.Delim('}') {
			return nil, ErrInvalidJSON
		}
		return obj, nil
	case json.Delim('['):
		arr := []any{}
		for d.More() {
			value, err := readValue(d, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			arr = append(arr, value)
		}
		if end, err := d.Token(); err != nil || end != json.Delim(']') {
			return nil, ErrInvalidJSON
		}
		return arr, nil
	default:
		switch token.(type) {
		case nil, string, bool, json.Number:
			return token, nil
		default:
			return nil, ErrInvalidJSON
		}
	}
}

// Redact masks all scalar values, including numeric identifiers, and every
// object key. Keys can themselves contain credentials or customer identifiers.
// Array positions and missing/null distinctions remain visible. This deliberately
// sacrifices semantic verification; the observation records that gap explicitly.
func Redact(value any) any {
	switch value := value.(type) {
	case map[string]any:
		// A list of member shapes avoids leaking untrusted property names. Sort
		// shapes for deterministic output without retaining original key order.
		members := make([]any, 0, len(value))
		for _, child := range value {
			members = append(members, Redact(child))
		}
		sortShapes(members)
		return map[string]any{"kind": "object", "members": members}
	case []any:
		items := make([]any, len(value))
		for i, child := range value {
			items[i] = Redact(child)
		}
		return map[string]any{"kind": "array", "items": items}
	case json.Number:
		return map[string]any{"kind": "number", "value": "[REDACTED]"}
	case string:
		return map[string]any{"kind": "string", "value": "[REDACTED]"}
	case bool:
		return map[string]any{"kind": "boolean", "value": "[REDACTED]"}
	default:
		return map[string]any{"kind": "null"}
	}
}
