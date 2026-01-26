package config

import (
	"context"
	"log/slog"

	"github.com/urfave/cli/v3"
	"github.com/warpcomdev/sixpack/internal/dispatcher"
	"github.com/warpcomdev/sixpack/internal/reloader"
)

type Reload struct {
	DryRun bool
}

func (c *Reload) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:     "dry-run",
			Usage:    "run pipeline without writing config",
			Sources:  cli.EnvVars("SIXPACK_DRY_RUN"),
			Category: "Runtime",
		},
	}
}

func (c *Reload) Apply(ctx *cli.Command) {
	c.DryRun = ctx.Bool("dry-run")
}

func (reloadCfg Reload) BuildReloader(logger *slog.Logger, apisixCfg APISIX, pluginCfg Plugins) (dispatcher.Reloader, error) {
	// Wire reloader (or dry-run).
	configPath := apisixCfg.ConfigPath(pluginCfg.EnableYAML)
	var reloadTarget dispatcher.Reloader
	if reloadCfg.DryRun {
		reloadTarget = &dryRunReloader{}
	} else {
		reloadTarget = &reloader.Reloader{
			ConfigPath: configPath,
			Logger:     logger,
		}
	}
	return reloadTarget, nil
}

// dryRunReloader is a no-op reloader used for dry-run mode.
type dryRunReloader struct{}

// Apply logs the payload size without making changes.
func (r *dryRunReloader) Apply(_ context.Context, payload []byte, useApi bool) error {
	return nil
}
