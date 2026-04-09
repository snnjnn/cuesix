package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"maps"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/warpcondev/cuesix/internal/compiler"
)

type envEnumerator struct {
	logger      *slog.Logger
	enumerator  compiler.Enumerator
	envFilename string
}

// NewEnvEnumerator wraps enumeration with APISIX-style env substitution.
func NewEnvEnumerator(logger *slog.Logger, enumerator compiler.Enumerator, envFilename string) (compiler.Enumerator, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if enumerator == nil {
		return nil, errors.New("Enumerator cannot be nil")
	}
	return envEnumerator{
		logger:      logger,
		envFilename: envFilename,
		enumerator:  enumerator,
	}, nil
}

// Enumerate delegates enumeration and applies environment substitution.
func (e envEnumerator) Enumerate(roots ...compiler.InputRoot) iter.Seq2[compiler.Source, error] {
	return EnvEnumerate(e.logger, e.envFilename, e.enumerator.Enumerate(roots...))
}

// EnvEnumerate substitutes placeholders using process env and optional env files.
func EnvEnumerate(logger *slog.Logger, envFilename string, sources iter.Seq2[compiler.Source, error]) iter.Seq2[compiler.Source, error] {
	if logger == nil {
		logger = slog.Default()
	}
	return func(yield func(compiler.Source, error) bool) {
		baseEnv := loadEnvironment()
		envCache := map[string]envCacheEntry{}
		for source, err := range sources {
			if err != nil {
				if !yield(source, err) {
					return
				}
				continue
			}
			envVars := baseEnv
			if envFilename != "" {
				dir := source.Ref.Dir()
				cacheKey := dir.Key()
				entry, exists := envCache[cacheKey]
				if !exists {
					entry.vars, entry.err = loadEnvVars(source.FS, source.Ref.Sibling(envFilename).Path, baseEnv)
					envCache[cacheKey] = entry
				}
				if entry.err != nil {
					if !yield(source, fmt.Errorf("env file %s: %w", source.Ref.Sibling(envFilename).Path, entry.err)) {
						return
					}
					continue
				}
				envVars = entry.vars
			}
			source.Data = []byte(compiler.SubstituteAPISIX(string(source.Data), envVars))
			if !yield(source, nil) {
				return
			}
		}
	}
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

func loadEnvVars(filesystem fs.FS, envPath string, baseEnv map[string]string) (map[string]string, error) {
	data, err := fs.ReadFile(filesystem, envPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return baseEnv, nil
		}
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
