package compiler

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
)

type Enumerator interface {
	Enumerate(...fs.FS) iter.Seq2[Source, error]
}

type BuiltinEnumerator struct {
	Logger *slog.Logger
}

func (be BuiltinEnumerator) Enumerate(fss ...fs.FS) iter.Seq2[Source, error] {
	return Enumerate(be.Logger, fss...)
}

func DefaultEnumerator(logger *slog.Logger, enumerator Enumerator) Enumerator {
	if enumerator == nil {
		return BuiltinEnumerator{Logger: logger}
	}
	return enumerator
}

func SubstituteAPISIX(input string, envVars map[string]string) string {
	return apisixEnvPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := apisixEnvPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		name := strings.TrimSpace(parts[1])
		defaultValue := ""
		if len(parts) > 2 {
			defaultValue = strings.TrimSpace(parts[2])
		}
		if name == "" {
			return defaultValue
		}
		value, ok := envVars[name]
		if ok {
			return value
		}
		return defaultValue
	})
}

func EnvEnumerate(logger *slog.Logger, envFilename string, sources iter.Seq2[Source, error]) iter.Seq2[Source, error] {
	if logger == nil {
		logger = slog.Default()
	}
	return func(yield func(Source, error) bool) {
		baseEnv := loadEnvironment()
		envCache := map[envCacheKey]envCacheEntry{}
		for source, err := range sources {
			if err != nil {
				if !yield(source, err) {
					return
				}
				continue
			}
			envVars := baseEnv
			if envFilename != "" {
				dir := path.Dir(source.Path)
				if cacheKey, ok := newEnvCacheKey(source.FS, dir); ok {
					entry, exists := envCache[cacheKey]
					if !exists {
						entry.vars, entry.err = loadEnvVars(source.FS, dir, envFilename, baseEnv)
						envCache[cacheKey] = entry
					}
					if entry.err != nil {
						if !yield(source, fmt.Errorf("env file %s: %w", path.Join(dir, envFilename), entry.err)) {
							return
						}
						continue
					}
					envVars = entry.vars
				} else {
					loadedVars, err := loadEnvVars(source.FS, dir, envFilename, baseEnv)
					if err != nil {
						if !yield(source, fmt.Errorf("env file %s: %w", path.Join(dir, envFilename), err)) {
							return
						}
						continue
					}
					envVars = loadedVars
				}
			}
			source.Data = []byte(SubstituteAPISIX(string(source.Data), envVars))
			if !yield(source, nil) {
				return
			}
		}
	}
}

type envCacheKey struct {
	filesystem fs.FS
	dir        string
}

type envCacheEntry struct {
	vars map[string]string
	err  error
}

var apisixEnvPattern = regexp.MustCompile(`\$\{\{\s*([A-Za-z_][A-Za-z0-9_]*)?\s*(?::=\s*([^}]*))?\s*\}\}`)

func newEnvCacheKey(filesystem fs.FS, dir string) (envCacheKey, bool) {
	filesystemType := reflect.TypeOf(filesystem)
	if filesystemType == nil || !filesystemType.Comparable() {
		return envCacheKey{}, false
	}
	return envCacheKey{filesystem: filesystem, dir: dir}, true
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

func loadEnvVars(filesystem fs.FS, dir, envPattern string, baseEnv map[string]string) (map[string]string, error) {
	envPath := path.Join(dir, envPattern)
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
	for key, value := range baseEnv {
		merged[key] = value
	}
	for key, value := range fileVars {
		merged[key] = value
	}
	return merged, nil
}

func parseEnvFile(data []byte) (map[string]string, error) {
	vars, err := godotenv.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return vars, nil
}
