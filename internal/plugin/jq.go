package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type JQPlugin struct {
	Runner jqRunner
}

type jqRunner interface {
	Run(input []byte, expression string) ([]byte, []byte, error)
}

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

func (p *JQPlugin) Update(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return value, nil
	}

	var root any
	if err := json.Unmarshal(value, &root); err != nil {
		return nil, &JQDecodeError{Err: err}
	}

	obj, ok := root.(map[string]any)
	if !ok {
		return value, nil
	}

	transforms, hasJQ, err := parseJQTransforms(obj)
	if err != nil {
		return nil, err
	}
	if !hasJQ {
		return value, nil
	}

	payload, err := json.Marshal(obj)
	if err != nil {
		return nil, &JQEncodeError{Err: err}
	}
	if len(transforms) == 0 {
		return payload, nil
	}

	sort.SliceStable(transforms, func(i, j int) bool {
		return transforms[i].Prio > transforms[j].Prio
	})

	runner := p.Runner
	if runner == nil {
		runner = systemJQRunner{}
	}
	expression := buildJQPipeline(transforms)
	stdout, stderr, err := runner.Run(payload, expression)
	if err != nil || len(stderr) > 0 {
		return nil, &JQExecError{
			Expr:   expression,
			Stderr: strings.TrimSpace(string(stderr)),
			Err:    err,
		}
	}
	return stdout, nil
}

type systemJQRunner struct{}

func (systemJQRunner) Run(input []byte, expression string) ([]byte, []byte, error) {
	cmd := exec.Command("jq", expression)
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func buildJQPipeline(transforms []JQTransform) string {
	parts := make([]string, 0, len(transforms))
	for _, transform := range transforms {
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
