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
	"path"
	"strings"

	"github.com/joho/godotenv"
	"github.com/warpcomdev/sixpack/internal/compiler"
)

type envEnumerator struct {
	logger      *slog.Logger
	enumerator  compiler.Enumerator
	envFilename string
}

func NewEnvEnumerator(logger *slog.Logger, enumerator compiler.Enumerator, envFilename string) compiler.Enumerator {
	if logger == nil {
		logger = slog.Default()
	}
	if enumerator == nil {
		enumerator = compiler.NewEnumerator(logger)
	}
	return envEnumerator{
		logger:      logger,
		envFilename: envFilename,
		enumerator:  enumerator,
	}
}

func (e envEnumerator) Enumerate(fss ...fs.FS) iter.Seq2[compiler.Source, error] {
	return EnvEnumerate(e.logger, e.envFilename, e.enumerator.Enumerate(fss...))
}

func EnvEnumerate(logger *slog.Logger, envFilename string, sources iter.Seq2[compiler.Source, error]) iter.Seq2[compiler.Source, error] {
	if logger == nil {
		logger = slog.Default()
	}
	return func(yield func(compiler.Source, error) bool) {
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
				cacheKey := envCacheKey{fsID: source.FSID, dir: dir}
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
			}
			source.Data = []byte(compiler.SubstituteAPISIX(string(source.Data), envVars))
			if !yield(source, nil) {
				return
			}
		}
	}
}

type envCacheKey struct {
	fsID int
	dir  string
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
