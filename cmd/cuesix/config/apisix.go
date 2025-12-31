package config

import (
	"time"

	"github.com/urfave/cli/v3"
	"github.com/warpcomdev/cuesix/internal/validator"
)

type APISIX struct {
	Home              string
	MirrorDir         string
	KeepMirror        bool
	ValidationTimeout time.Duration
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
	}
}

func (c *APISIX) Apply(ctx *cli.Command) {
	c.Home = ctx.String("apisix-home")
	c.MirrorDir = ctx.String("mirror-dir")
	c.KeepMirror = ctx.Bool("keep-mirror")
	c.ValidationTimeout = ctx.Duration("validation-timeout")
}

func (c APISIX) ConfigPath(outputYAML bool) string {
	return validator.BuildConfigPath(c.Home, outputYAML)
}
