// Package control implements the opt-in, synthetic-only Week 3 registry lab.
// It has no production authentication, ingress, publication or deployment path.
package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"time"
)

type Engine struct {
	Path string
}

// Call runs the offline Rust parser with a wall-clock limit and bounded output.
// The service serializes calls under its registry mutex; no unbounded job queue.
func (e Engine) Call(ctx context.Context, request any, result any) error {
	body, err := json.Marshal(request)
	if err != nil || len(body) > 256*1024 {
		return errors.New("contract engine input limit")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.Path)
	cmd.Stdin = bytes.NewReader(body)
	output := &boundedOutput{}
	cmd.Stdout, cmd.Stderr = output, io.Discard
	if err := cmd.Run(); err != nil {
		return errors.New("offline contract engine unavailable or resource limit")
	}
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if json.Unmarshal(output.Bytes(), &envelope) != nil {
		return errors.New("invalid engine response")
	}
	if !envelope.OK {
		return errors.New(envelope.Error)
	}
	if json.Unmarshal(envelope.Result, result) != nil {
		return errors.New("invalid engine result")
	}
	return nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2*1024*1024 {
		return 0, errors.New("engine output limit")
	}
	return b.Buffer.Write(p)
}
