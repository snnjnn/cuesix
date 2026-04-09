package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/validator"
	"github.com/urfave/cli/v3"
)

type Apisix struct {
	Home         string
	Virtualgw    string
	OutputYAML   bool
	EnableLabels bool
}

type StandaloneValidator struct {
	MaxGatewayDepth   int
	MirrorDir         string
	KeepMirror        bool
	ValidationTimeout time.Duration
	UseSchema         bool
}

// Flags returns APISIX runtime and validation command-line flags.
func (c *Apisix) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "apisix-virtual-gateway",
			Usage:    "default virtual gateway to use for this apisix instance",
			Sources:  cli.EnvVars("SIXPACK_APISIX_VIRTUAL_GATEWAY"),
			Value:    compiler.DEFAULT_VIRTUALGW,
			Category: "APISIX",
		},
		&cli.StringFlag{
			Name:     "apisix-home",
			Usage:    "apisix home path",
			Sources:  cli.EnvVars("SIXPACK_APISIX_HOME"),
			Value:    "/usr/local/apisix",
			Category: "APISIX",
		},
		&cli.BoolFlag{
			Name:     "plugin-yaml",
			Usage:    "enable yaml post-render plugin",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_YAML"),
			Category: "APISIX",
		},
		&cli.BoolFlag{
			Name:     "plugin-labels",
			Usage:    "add managed ownership labels to resources that support labels",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_LABELS"),
			Category: "APISIX",
		},
	}
}

// Apply loads APISIX configuration values from parsed CLI flags.
func (c *Apisix) Apply(ctx *cli.Command) {
	c.Home = ctx.String("apisix-home")
	c.Virtualgw = ctx.String("apisix-virtual-gateway")
	c.OutputYAML = ctx.Bool("plugin-yaml")
	c.EnableLabels = ctx.Bool("plugin-labels")
}

// Validate input flags values
func (c *Apisix) Validate() error {
	if strings.TrimSpace(c.Virtualgw) == "" {
		return errors.New("apisix-virtual-gateway must not be empty")
	}
	return nil
}

// Flags returns APISIX runtime and validation command-line flags.
func (c *StandaloneValidator) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "mirror-dir",
			Usage:    "apisix mirror directory (optional)",
			Sources:  cli.EnvVars("SIXPACK_MIRROR_DIR"),
			Category: "APISIX",
		},
		&cli.IntFlag{
			Name:     "max-gateway-depth",
			Usage:    "max number of virtual gateway separators (.) that are validated/readiness-tracked; 0=top-level only, 1=one level deep",
			Sources:  cli.EnvVars("SIXPACK_MAX_GATEWAY_DEPTH"),
			Value:    0,
			Category: "APISIX",
		},
		&cli.BoolFlag{
			Name:     "keep-mirror",
			Usage:    "Do not remove mirror on startup",
			Sources:  cli.EnvVars("SIXPACK_KEEP_MIRROR"),
			Category: "APISIX",
		},
		&cli.DurationFlag{
			Name:     "validation-timeout",
			Usage:    "timeout for apisix test",
			Sources:  cli.EnvVars("SIXPACK_VALIDATION_TIMEOUT"),
			Value:    30 * time.Second,
			Category: "APISIX",
		},
		&cli.BoolFlag{
			Name:     "apisix-use-schema",
			Usage:    "validate config snippets against APISIX schema (requires --apisix-control-url)",
			Sources:  cli.EnvVars("SIXPACK_APISIX_USE_SCHEMA"),
			Category: "APISIX",
		},
	}
}

// Apply loads APISIX configuration values from parsed CLI flags.
func (c *StandaloneValidator) Apply(ctx *cli.Command) {
	c.MirrorDir = ctx.String("mirror-dir")
	c.KeepMirror = ctx.Bool("keep-mirror")
	c.ValidationTimeout = ctx.Duration("validation-timeout")
	c.UseSchema = ctx.Bool("apisix-use-schema")
	c.MaxGatewayDepth = ctx.Int("max-gateway-depth")
}

// Validate input flags values
func (c *StandaloneValidator) Validate() error {
	if c.MaxGatewayDepth < 0 {
		return errors.New("max-gateway-depth must be >= 0")
	}
	return nil
}

// BuildValidator creates a validator backed by an APISIX mirror directory.
func (c *StandaloneValidator) BuildValidator(logger *slog.Logger, home string) (zero validator.Validator, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	// APISIX validation and mirror setup.
	home = strings.TrimSpace(home)
	if home == "" {
		logger.Error("missing apisix home path")
		return zero, errors.New("missing apisix home path")
	}
	mirrorKeep := c.KeepMirror
	mirrorDir := c.MirrorDir
	if mirrorDir == "" {
		tmp, tmpErr := os.MkdirTemp("", "sixpack-apisix-")
		if tmpErr != nil {
			return zero, fmt.Errorf("create apisix mirror dir failed: %w", tmpErr)
		}
		mirrorKeep = false // no need to recreate it
		mirrorDir = tmp
	}
	return validator.New(logger, home, mirrorDir, mirrorKeep, c.ValidationTimeout, nil)
}
