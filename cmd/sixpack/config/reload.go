package config

import (
	"context"
	"log/slog"

	"github.com/urfave/cli/v3"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/reloader"
)

type Reload struct {
	DryRun bool
}

// Flags returns runtime reloader command-line flags.
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

// Apply loads reload settings from parsed CLI flags.
func (c *Reload) Apply(ctx *cli.Command) {
	c.DryRun = ctx.Bool("dry-run")
}

// BuildReloader builds either a real reloader or a dry-run no-op reloader.
func (c *Reload) BuildReloader(logger *slog.Logger, virtualgw string, configPath string) (dispatcher.Reloader, error) {
	// Wire reloader (or dry-run).
	var reloadTarget dispatcher.Reloader
	if c.DryRun {
		reloadTarget = dryRunReloader{}
	} else {
		reloadTarget = &reloader.FileReloader{
			Virtualgw:  virtualgw,
			ConfigPath: configPath,
			Logger:     logger,
		}
	}
	return reloadTarget, nil
}

// Validate input flags values
func (c *Reload) Validate() error {
	return nil
}

// dryRunReloader is a no-op reloader used for dry-run mode.
type dryRunReloader struct{}

// Apply logs the payload size without making changes.
func (r dryRunReloader) Apply(_ context.Context, virtualgw string, payload []byte) error {
	return nil
}

// DryRunValidator is a no-op validator used for dry-run mode.
type DryRunValidator struct{}

// Apply logs the payload size without making changes.
func (r DryRunValidator) Validate(candidate []byte, isYAML bool) (bool, error) {
	return true, nil
}
