package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"github.com/warpcomdev/cuesix/internal/validator"
)

type APISIX struct {
	Home              string
	MirrorDir         string
	KeepMirror        bool
	ValidationTimeout time.Duration
	UseSchema         bool
}

func (c *APISIX) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "apisix-home",
			Usage:    "apisix home path",
			Sources:  cli.EnvVars("CUESIX_APISIX_HOME"),
			Value:    "/usr/local/apisix",
			Category: "APISIX",
		},
		&cli.StringFlag{
			Name:     "mirror-dir",
			Usage:    "apisix mirror directory (optional)",
			Sources:  cli.EnvVars("CUESIX_MIRROR_DIR"),
			Category: "APISIX",
		},
		&cli.BoolFlag{
			Name:     "keep-mirror",
			Usage:    "Do not remove mirror on startup",
			Sources:  cli.EnvVars("CUESIX_KEEP_MIRROR"),
			Category: "APISIX",
		},
		&cli.DurationFlag{
			Name:     "validation-timeout",
			Usage:    "timeout for apisix test",
			Sources:  cli.EnvVars("CUESIX_VALIDATION_TIMEOUT"),
			Value:    30 * time.Second,
			Category: "APISIX",
		},
		&cli.BoolFlag{
			Name:     "apisix-use-schema",
			Usage:    "validate config snippets against APISIX schema (requires --apisix-control-url)",
			Sources:  cli.EnvVars("CUESIX_APISIX_USE_SCHEMA"),
			Category: "APISIX",
		},
	}
}

func (c *APISIX) Apply(ctx *cli.Command) {
	c.Home = ctx.String("apisix-home")
	c.MirrorDir = ctx.String("mirror-dir")
	c.KeepMirror = ctx.Bool("keep-mirror")
	c.ValidationTimeout = ctx.Duration("validation-timeout")
	c.UseSchema = ctx.Bool("apisix-use-schema")
}

func (c APISIX) ConfigPath(outputYAML bool) string {
	return validator.BuildConfigPath(c.Home, outputYAML)
}

func (c APISIX) BuildValidator(logger *slog.Logger) (zero validator.Validator, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	// APISIX validation and mirror setup.
	if strings.TrimSpace(c.Home) == "" {
		logger.Error("missing apisix home path")
		return zero, errors.New("missing apisix home path")
	}
	mirrorKeep := c.KeepMirror
	mirrorDir := c.MirrorDir
	if mirrorDir == "" {
		tmp, tmpErr := os.MkdirTemp("", "cuesix-apisix-")
		if tmpErr != nil {
			return zero, fmt.Errorf("create apisix mirror dir failed: %w", tmpErr)
		}
		mirrorKeep = false // no need to recreate it
		mirrorDir = tmp
	}
	return validator.New(logger, c.Home, mirrorDir, mirrorKeep, c.ValidationTimeout, nil)
}
