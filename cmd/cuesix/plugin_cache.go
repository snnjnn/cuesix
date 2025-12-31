package main

import (
	"log/slog"
	"time"

	"github.com/warpcomdev/cuesix/internal/cache"
	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

// pluginCache runs plugin chains with cache normalization.
type pluginCache struct {
	preRender  plugin.PreRender
	postRender plugin.PostRender
	cache      *cache.Cache
}

// Changed runs pre-render plugins, cache normalization, and post-render plugins.
func (p *pluginCache) Changed(logger *slog.Logger, value map[string]any) ([]byte, error) {
	logger.Info("pre-render plugins start")
	updated, err := p.preRender.Update(logger, value)
	if err != nil {
		return nil, err
	}
	logger.Info("pre-render plugins complete")
	logger.Info("cache normalization start")
	result, err := p.cache.Changed(logger, updated)
	if result == nil || err != nil {
		return nil, err
	}
	logger.Info("cache normalization complete")
	logger.Info("post-render plugins start")
	output, err := p.postRender.Update(logger, result)
	if err != nil {
		return nil, err
	}
	logger.Info("post-render plugins complete")
	return output, nil
}

// buildPreRender constructs the pre-render plugin chain.
func buildPreRender(sslPaths []string, acmeWatcher *certmagicmgr.Watcher, fallback certmagicmgr.Certificate) (plugin.PreRender, error) {
	var plugins plugin.PreRenderChain
	if len(sslPaths) > 0 {
		sslFSes, err := buildFilesystems(sslPaths)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, &ssl.SSLPlugin{
			Filesystems: sslFSes,
			ACME:        acmeWatcher,
			Fallback:    fallback,
		})
	}
	return plugins, nil
}

// buildPostRender constructs the post-render plugin chain.
func buildPostRender(enableJQ bool, enableYAML bool, jqTimeout time.Duration) (plugin.PostRender, error) {
	var plugins plugin.PostRenderChain
	if enableJQ {
		plugins = append(plugins, &plugin.JQPlugin{Timeout: jqTimeout})
	}
	if enableYAML {
		// YAMLPlugin siempre debe ser el ultimo plugin.
		plugins = append(plugins, &plugin.YAMLPlugin{})
	}
	return plugins, nil
}
