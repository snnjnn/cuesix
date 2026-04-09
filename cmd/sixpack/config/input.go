package config

import (
	"errors"
	"time"

	"github.com/urfave/cli/v3"
)

type Input struct {
	InputDirs       []string
	GatewayFromDots bool
	Cooldown        time.Duration
}

// Flags returns input-source and cooldown command-line flags.
func (c *Input) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.DurationFlag{
			Name:     "cooldown",
			Usage:    "cooldown duration",
			Sources:  cli.EnvVars("SIXPACK_COOLDOWN"),
			Value:    0,
			Category: "Input",
		},
		&cli.StringSliceFlag{
			Name:     "input",
			Usage:    "input directory (repeatable)",
			Sources:  cli.EnvVars("SIXPACK_INPUT_DIRS"),
			Category: "Input",
		},
		&cli.BoolFlag{
			Name:     "virtualgw-from-dots",
			Usage:    "Derive the virtualgateway name from directory basename (without extension)",
			Sources:  cli.EnvVars("SIXPACK_VIRTUALGW_FROM_DOTS"),
			Category: "Input",
		},
	}
}

// Apply loads input configuration from parsed CLI flags.
func (c *Input) Apply(ctx *cli.Command) {
	c.Cooldown = ctx.Duration("cooldown")
	c.GatewayFromDots = ctx.Bool("virtualgw-from-dots")
	c.InputDirs = ctx.StringSlice("input")
}

// Validate ensures at least one input directory is configured.
func (c *Input) Validate() error {
	if len(c.InputDirs) == 0 {
		return errors.New("at least one --input or SIXPACK_INPUT_DIRS is required")
	}
	return nil
}
