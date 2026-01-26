package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/warpcomdev/sixpack/internal/runner"
)

// JQPlugin applies a jq pipeline defined in the top-level jq list.
type JQPlugin struct {
	Runner  Runner
	Timeout time.Duration
}

type Runner interface {
	RunCommand(ctx context.Context, workDir string, input []byte, name string, args ...string) ([]byte, []byte, error)
}

// JQTransform describes a single jq expression in the pipeline.
type JQTransform struct {
	ID   string `json:"id"`
	Prio int    `json:"prio"`
	Expr string `json:"expr"`
}

type JQDecodeError struct {
	Err error
}

func (e *JQDecodeError) Error() string {
	return fmt.Sprintf("jq decode failed: %s", e.Err.Error())
}

func (e *JQDecodeError) Unwrap() error {
	return e.Err
}

type JQEncodeError struct {
	Err error
}

func (e *JQEncodeError) Error() string {
	return fmt.Sprintf("jq encode failed: %s", e.Err.Error())
}

func (e *JQEncodeError) Unwrap() error {
	return e.Err
}

type JQExecError struct {
	Expr   string
	Stderr string
	Err    error
}

func (e *JQExecError) Error() string {
	if e.Err != nil && e.Stderr != "" {
		return fmt.Sprintf("jq execution failed: %s: %s", e.Err.Error(), e.Stderr)
	}
	if e.Err != nil {
		return fmt.Sprintf("jq execution failed: %s", e.Err.Error())
	}
	if e.Stderr != "" {
		return fmt.Sprintf("jq execution failed: %s", e.Stderr)
	}
	return "jq execution failed"
}

func (e *JQExecError) Unwrap() error {
	return e.Err
}

type JQConfigError struct {
	Field string
	Err   error
}

func (e *JQConfigError) Error() string {
	if e.Field == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Err.Error())
}

func (e *JQConfigError) Unwrap() error {
	return e.Err
}

func (p *JQPlugin) Update(logger *slog.Logger, value []byte) ([]byte, error) {
	if len(value) == 0 {
		return value, nil
	}

	logger.Info("jq plugin start", "bytes", len(value))

	var root any
	if err := json.Unmarshal(value, &root); err != nil {
		logger.Error("jq plugin decode failed", "error", err)
		return nil, &JQDecodeError{Err: err}
	}

	obj, ok := root.(map[string]any)
	if !ok {
		return value, nil
	}

	transforms, hasJQ, err := parseJQTransforms(obj)
	if err != nil {
		logger.Error("jq plugin config invalid", "error", err)
		return nil, err
	}
	if !hasJQ {
		logger.Info("jq plugin skipped: no jq config")
		return value, nil
	}

	payload, err := json.Marshal(obj)
	if err != nil {
		logger.Error("jq plugin encode failed", "error", err)
		return nil, &JQEncodeError{Err: err}
	}
	if len(transforms) == 0 {
		logger.Info("jq plugin no transforms", "bytes", len(payload))
		return payload, nil
	}

	sort.SliceStable(transforms, func(i, j int) bool {
		return transforms[i].Prio > transforms[j].Prio
	})

	r := p.Runner
	if r == nil {
		r = runner.New()
	}
	expression := buildJQPipeline(logger, transforms)
	logger.Info("jq plugin executing", "transforms", len(transforms))
	ctx := context.Background()
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}
	stdout, stderr, err := r.RunCommand(ctx, "", payload, "jq", expression)
	if err != nil || len(stderr) > 0 {
		logger.Error("jq plugin execution failed", "error", err, "stderr", slog.String("stderr", strings.TrimSpace(string(stderr))))
		return nil, &JQExecError{
			Expr:   expression,
			Stderr: strings.TrimSpace(string(stderr)),
			Err:    err,
		}
	}
	logger.Info("jq plugin complete", "bytes", len(stdout))
	return stdout, nil
}

func buildJQPipeline(logger *slog.Logger, transforms []JQTransform) string {
	parts := make([]string, 0, len(transforms))
	for _, transform := range transforms {
		logger.Info("jq plugin appending transform", "id", transform.ID, "prio", transform.Prio)
		parts = append(parts, "("+transform.Expr+")")
	}
	return strings.Join(parts, " | ")
}

func parseJQTransforms(root map[string]any) ([]JQTransform, bool, error) {
	raw, ok := root["jq"]
	if !ok {
		return nil, false, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, true, &JQConfigError{Field: "jq", Err: err}
	}
	var transforms []JQTransform
	decoder := json.NewDecoder(bytes.NewBuffer(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&transforms); err != nil {
		return nil, true, &JQConfigError{Field: "jq", Err: err}
	}
	for idx, transform := range transforms {
		if strings.TrimSpace(transform.Expr) == "" {
			return nil, true, &JQConfigError{Field: fmt.Sprintf("jq[%d].expr", idx), Err: errors.New("expression is required")}
		}
	}
	delete(root, "jq")
	return transforms, true, nil
}
