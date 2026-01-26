package factory

import (
	"context"
	"hash/fnv"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/warpcomdev/sixpack/cmd/sixpack/config"
	"github.com/warpcomdev/sixpack/internal/cursor"
	"github.com/warpcomdev/sixpack/internal/plugin"
	"github.com/warpcomdev/sixpack/internal/plugin/ssl"
	"github.com/warpcomdev/sixpack/internal/serializer"
)

type SerializerFactory struct {
	// Plugin configurations
	sslPlugin *ssl.SSLPlugin
	preCache  plugin.PreRender
	postCache plugin.PostRender
	logger    *slog.Logger
	// Setup references
	sslSetup  SSLSetup
	scheduler *Scheduler
	// Global record of the last committed config
	commitHash uint64
	hasCommit  bool
	// Global records of last committed certs
	CommittedCerts map[ssl.Tracking]time.Time
}

func NewSerializer(logger *slog.Logger, cfg config.Plugins, sslSetup SSLSetup, scheduler *Scheduler) (SerializerFactory, error) {
	sf := SerializerFactory{
		sslSetup:  sslSetup,
		scheduler: scheduler,
		logger:    logger,
	}
	if err := sf.buildPreRender(cfg); err != nil {
		return sf, err
	}
	if err := sf.buildPostRender(cfg); err != nil {
		return sf, err
	}
	return sf, nil
}

// Instance spawns a new instance of plugin factory
func (p *SerializerFactory) Instance() *SerializerInstance {
	return &SerializerInstance{
		SerializerFactory: p,
	}
}

type SerializerInstance struct {
	*SerializerFactory
	record map[ssl.Tracking]time.Time
	hash   uint64
}

func (p *SerializerInstance) Reset() {
	// Start a new recording track
	p.record = make(map[ssl.Tracking]time.Time)
	p.hash = 0
}

func (p *SerializerInstance) Commit() {
	// Record the winning hash
	p.commitHash = p.hash
	p.hasCommit = true
	// Record the winning certs
	p.CommittedCerts = p.record
}

// Changed runs pre-render plugins, cache normalization, and post-render plugins.
func (p *SerializerInstance) Serialize(value map[string]any) ([]byte, error) {
	var err error
	logger := p.logger
	if p.sslPlugin != nil {
		logger.Info("SSL Plugins start")
		value, err = p.sslPlugin.Update(context.Background(), value, p.record)
	}
	if err != nil {
		return nil, err
	}
	logger.Info("pre-serialize plugins start")
	value, err = p.preCache.Update(logger, value)
	if err != nil {
		return nil, err
	}
	logger.Info("serialization start")
	result, err := serializer.Serialize(value)
	if err != nil {
		return nil, err
	}
	// Update the hash
	p.hash = hashBytes(result)
	if p.hasCommit && p.hash == p.commitHash {
		logger.Info("serialization cache hit")
		return nil, nil
	}
	logger.Info("post-cache plugins start")
	output, err := p.postCache.Update(logger, result)
	if err != nil {
		return nil, err
	}
	logger.Info("post-cache plugins complete")
	return output, nil
}

func hashBytes(payload []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(payload)
	return h.Sum64()
}

// buildPreRender constructs the pre-render plugin chain.
func (p *SerializerFactory) buildPreRender(cfg config.Plugins) error {
	// SSL Plugin is handled differently because so far, is the only
	// plugin with state (records requested certs)
	if cfg.EnableSSL {
		p.sslPlugin = &ssl.SSLPlugin{
			LiveHandler: ssl.LiveHandler{
				RequestTimeout: cfg.SSLACMETimeout,
				Tracker:        p.sslSetup.AcmeTracker,
			},
			Fallback: p.sslSetup.FallbackCert,
			Logger:   p.logger,
		}
	}
	// Other pre-cache plugins with the standard interface
	var preCache plugin.PreRenderChain
	p.preCache = preCache
	return nil
}

// buildPostRender constructs the post-render plugin chain.
func (p *SerializerFactory) buildPostRender(cfg config.Plugins) error {
	var plugins plugin.PostRenderChain
	if cfg.EnableJQ {
		plugins = append(plugins, &plugin.JQPlugin{Timeout: cfg.JQTimeout})
	}
	if cfg.EnableYAML {
		// YAMLPlugin must always be the last plugin.
		plugins = append(plugins, &plugin.YAMLPlugin{})
	}
	p.postCache = plugins
	return nil
}

// BuildFilesystems creates read-only filesystems for the input paths.
func BuildFilesystems(paths []string) ([]fs.FS, error) {
	fses := make([]fs.FS, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, err := os.Stat(clean); err != nil {
			return nil, err
		}
		fses = append(fses, os.DirFS(clean))
	}
	return fses, nil
}

// Expires certificates no longer used
func (s SerializerFactory) loop(groupCtx context.Context, cleanupInterval time.Duration, task func()) {
	timer := time.NewTicker(cleanupInterval)
	defer timer.Stop()
	for range cursor.All(groupCtx, cursor.Channel(timer.C)) {
		s.scheduler.Might(groupCtx, task)
	}
}

// Expires certificates no longer used
func (s SerializerFactory) ExpireLoop(groupCtx context.Context, logger *slog.Logger, cleanupInterval, expiredGrace time.Duration) {
	s.loop(groupCtx, cleanupInterval, func() {
		if err := s.sslSetup.AcmeManager.RemoveExpired(groupCtx, cleanupInterval, expiredGrace); err != nil {
			logger.Error("error removing expired certs", "error", err)
		}
	})
}

// Updates certificates recently renewed
func (s SerializerFactory) CommitLoop(groupCtx context.Context, logger *slog.Logger, cleanupInterval, untrackedGrace time.Duration) {
	s.loop(groupCtx, cleanupInterval, func() {
		if len(s.CommittedCerts) == 0 {
			return
		}
		s.sslSetup.AcmeTracker.Commit(groupCtx, logger, s.CommittedCerts, untrackedGrace)
	})
}
