package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/joho/godotenv"
	"github.com/warpcomdev/cuesix/internal/compiler"
)

type envInput struct {
	logger      *slog.Logger
	input       compiler.Input
	envFilename string
	baseEnv     map[string]string
	envCache    map[string]map[string]envCacheEntry
}

// NewEnvInput wraps input with APISIX-style env substitution.
func NewEnvInput(logger *slog.Logger, input compiler.Input, envFilename string) (compiler.Input, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if input == nil {
		return nil, errors.New("input cannot be nil")
	}
	return &envInput{
		logger:      logger,
		input:       input,
		envFilename: envFilename,
		baseEnv:     loadEnvironment(),
		envCache:    make(map[string]map[string]envCacheEntry),
	}, nil
}

// Namespaces implements compiler.Input.
func (e *envInput) Namespaces() []string {
	namespaces := e.input.Namespaces()
	// Clear obsolete cache entries for namespaces that no longer exist in the input
	for ns := range e.envCache {
		if !slices.Contains(namespaces, ns) {
			delete(e.envCache, ns)
		}
	}
	return namespaces
}

// Enumerate implements compiler.Input.
func (e *envInput) Enumerate(namespace string) iter.Seq2[compiler.SourceRef, error] {
	e.envCache[namespace] = make(map[string]envCacheEntry) // reset cache on each enumeration
	return e.input.Enumerate(namespace)
}

// Open implements compiler.Input
func (e *envInput) Open(ref compiler.SourceRef) (io.ReadCloser, error) {
	logger := e.logger
	if logger == nil {
		logger = slog.Default()
	}
	envVars := e.baseEnv
	if e.envFilename != "" {
		dir := ref.Dir()
		cacheKey := dir.Key()
		envCache, exists := e.envCache[ref.Namespace]
		if exists {
			entry, exists := envCache[cacheKey]
			if !exists {
				entry.vars, entry.err = loadEnvVars(e.input, ref.Sibling(e.envFilename), e.baseEnv)
				envCache[cacheKey] = entry
			}
			if entry.err != nil {
				return nil, fmt.Errorf("env file %s: %w", ref.Sibling(e.envFilename).Path, entry.err)
			}
			envVars = entry.vars
		}
	}
	data, err := e.input.Open(ref)
	if err != nil {
		return nil, err
	}
	defer data.Close()
	payload, err := io.ReadAll(data)
	if err != nil {
		return nil, err
	}
	replaced := compiler.SubstituteAPISIX(string(payload), envVars)
	return io.NopCloser(strings.NewReader(replaced)), nil
}

type envCacheEntry struct {
	vars map[string]string
	err  error
}

func loadEnvironment() map[string]string {
	envVars := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		envVars[key] = value
	}
	return envVars
}

func loadEnvVars(input compiler.Input, envPath compiler.SourceRef, baseEnv map[string]string) (map[string]string, error) {
	reader, err := input.Open(envPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return baseEnv, nil
		}
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	fileVars, err := parseEnvFile(data)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]string, len(baseEnv)+len(fileVars))
	maps.Copy(merged, baseEnv)
	maps.Copy(merged, fileVars)
	return merged, nil
}

func parseEnvFile(data []byte) (map[string]string, error) {
	vars, err := godotenv.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return vars, nil
}
