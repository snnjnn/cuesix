package factory

import (
	"context"
	"hash/fnv"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/warpcondev/cuesix/cmd/sixpack/config"
	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/cursor"
	"github.com/warpcondev/cuesix/internal/dispatcher"
	"github.com/warpcondev/cuesix/internal/plugin"
	"github.com/warpcondev/cuesix/internal/plugin/ssl"
	"github.com/warpcondev/cuesix/internal/serializer"
)

type SerializerFactory struct {
	// Plugin configurations
	sslPlugin *ssl.SSLPlugin
	preCache  plugin.PreRender
	postCache plugin.PostRender
	rules     compiler.MergingRule
	addLabels bool
	logger    *slog.Logger
	// Setup references
	sslSetup  SSLSetup
	scheduler *Scheduler
	// serializer instances
	instances map[string]*SerializerInstance
}

// NewSerializer builds a SerializerFactory with pre/post-render pipelines
// configured from plugin flags.
func NewSerializer(logger *slog.Logger, pluginCfg config.Plugins, apisixConfig config.Apisix, sslSetup SSLSetup, scheduler *Scheduler) (SerializerFactory, error) {
	sf := SerializerFactory{
		sslSetup:  sslSetup,
		scheduler: scheduler,
		logger:    logger,
		instances: make(map[string]*SerializerInstance),
		rules:     compiler.DefaultMergingRules(),
	}
	if err := sf.buildPreRender(pluginCfg, apisixConfig); err != nil {
		return sf, err
	}
	if err := sf.buildPostRender(apisixConfig); err != nil {
		return sf, err
	}
	return sf, nil
}

func (sf *SerializerFactory) allCommittedCerts() map[ssl.Tracking]time.Time {
	allCommittedCerts := make(map[ssl.Tracking]time.Time)
	for _, instance := range sf.instances {
		for tracking, committedAt := range instance.CommittedCerts {
			existing, ok := allCommittedCerts[tracking]
			if !ok || committedAt.After(existing) {
				allCommittedCerts[tracking] = committedAt
			}
		}
	}
	return allCommittedCerts
}

// Single serializer instance for a particular virtual gateway
type SerializerInstance struct {
	logger    *slog.Logger
	virtualgw string
	// Prebuilt plugin instances
	sslPlugin *ssl.SSLPlugin
	preCache  plugin.PreRender
	postCache plugin.PostRender
	// Global record of the last committed config
	commitHash uint64
	hasCommit  bool
	// Global records of last committed certs
	CommittedCerts map[ssl.Tracking]time.Time
	record         map[ssl.Tracking]time.Time
	hash           uint64
}

// Instance creates a per-run serializer instance with isolated state tracking.
func (p *SerializerFactory) Instance(virtualgw string) dispatcher.Serializer {
	instance := &SerializerInstance{
		logger:    p.logger,
		virtualgw: virtualgw,
		sslPlugin: p.sslPlugin,
		preCache:  p.preCache,
		postCache: p.postCache,
	}
	// Plugins that cannot be prevbuilt, because they depend
	// on the virtual gateway instance we are rendering.
	if p.addLabels {
		instance.preCache = plugin.PreRenderChain{
			&plugin.ManagedLabelsPlugin{
				VirtualGateway: virtualgw,
				Rules:          p.rules,
			},
			p.preCache,
		}
	}
	p.instances[virtualgw] = instance
	return instance
}

// Reset clears per-run state before processing a new dispatch cycle.
func (p *SerializerInstance) Reset() {
	// Start a new recording track
	p.record = make(map[ssl.Tracking]time.Time)
	p.hash = 0
}

// Commit stores the last successful hash and certificate tracking snapshot.
func (p *SerializerInstance) Commit() {
	// Record the winning hash
	p.commitHash = p.hash
	p.hasCommit = true
	// Record the winning certs
	p.CommittedCerts = p.record
}

// Serialize runs SSL/pre-render plugins, serializes the config, applies
// post-render plugins, and returns nil when output is unchanged (cache hit).
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
func (p *SerializerFactory) buildPreRender(cfg config.Plugins, apisixCfg config.Apisix) error {
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
	p.addLabels = apisixCfg.EnableLabels
	p.preCache = preCache
	return nil
}

// buildPostRender constructs the post-render plugin chain.
func (p *SerializerFactory) buildPostRender(cfg config.Apisix) error {
	var plugins plugin.PostRenderChain
	if cfg.OutputYAML {
		// YAMLPlugin must always be the last plugin.
		plugins = append(plugins, &plugin.YAMLPlugin{})
	}
	p.postCache = plugins
	return nil
}

// BuildFilesystems creates read-only filesystems for the input paths.
func BuildFilesystems(paths []string) ([]compiler.InputRoot, error) {
	roots := make([]compiler.InputRoot, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, err := os.Stat(clean); err != nil {
			return nil, err
		}
		roots = append(roots, compiler.InputRoot{
			Name: clean,
			FS:   os.DirFS(clean),
		})
	}
	return roots, nil
}

// loop runs a periodic task using the factory scheduler.
func (s SerializerFactory) loop(groupCtx context.Context, cleanupInterval time.Duration, task func()) {
	timer := time.NewTicker(cleanupInterval)
	defer timer.Stop()
	for range cursor.All(groupCtx, cursor.Channel(timer.C)) {
		s.scheduler.Might(groupCtx, task)
	}
}

// ExpireLoop periodically removes expired certificates from storage.
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
		allCommittedCerts := make(map[ssl.Tracking]time.Time)
		for _, instance := range s.instances {
			if len(instance.CommittedCerts) > 0 {
				maps.Copy(allCommittedCerts, instance.CommittedCerts)
			}
		}
		if len(allCommittedCerts) == 0 {
			return
		}
		s.sslSetup.AcmeTracker.Commit(groupCtx, logger, allCommittedCerts, untrackedGrace)
	})
}
