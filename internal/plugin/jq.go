package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// JQPlugin applies a jq pipeline defined in the top-level jq list.
type JQPlugin struct {
	Runner  jqRunner
	Timeout time.Duration
}

type jqRunner interface {
	Run(ctx context.Context, input []byte, expression string) ([]byte, []byte, error)
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
	Err   string
}

func (e *JQConfigError) Error() string {
	if e.Field == "" {
		return e.Err
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Err)
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

	runner := p.Runner
	if runner == nil {
		runner = systemJQRunner{}
	}
	expression := buildJQPipeline(logger, transforms)
	logger.Info("jq plugin executing", "transforms", len(transforms))
	ctx := context.Background()
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}
	stdout, stderr, err := runner.Run(ctx, payload, expression)
	if err != nil || len(stderr) > 0 {
		logger.Error("jq plugin execution failed", "error", err, "stderr", strings.TrimSpace(string(stderr)))
		return nil, &JQExecError{
			Expr:   expression,
			Stderr: strings.TrimSpace(string(stderr)),
			Err:    err,
		}
	}
	logger.Info("jq plugin complete", "bytes", len(stdout))
	return stdout, nil
}

type systemJQRunner struct{}

func (systemJQRunner) Run(ctx context.Context, input []byte, expression string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "jq", expression)
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
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
		return nil, true, &JQConfigError{Field: "jq", Err: "invalid content"}
	}
	var transforms []JQTransform
	decoder := json.NewDecoder(bytes.NewBuffer(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&transforms); err != nil {
		return nil, true, &JQConfigError{Field: "jq", Err: "invalid content"}
	}
	for idx, transform := range transforms {
		if strings.TrimSpace(transform.Expr) == "" {
			return nil, true, &JQConfigError{Field: fmt.Sprintf("jq[%d].expr", idx), Err: "is required"}
		}
	}
	delete(root, "jq")
	return transforms, true, nil
}
