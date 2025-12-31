package config

import (
	"errors"
	"time"

	"github.com/urfave/cli/v2"
)

type Input struct {
	InputDirs []string
	Cooldown  time.Duration
}

func (c *Input) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.DurationFlag{
			Name:     "cooldown",
			Usage:    "cooldown duration",
			EnvVars:  []string{"CUESIX_COOLDOWN"},
			Value:    0,
			Category: "Input",
		},
		&cli.StringSliceFlag{
			Name:     "input",
			Usage:    "input directory (repeatable)",
			EnvVars:  []string{"CUESIX_INPUT_DIRS"},
			Category: "Input",
		},
	}
}

func (c *Input) Apply(ctx *cli.Context) {
	c.Cooldown = ctx.Duration("cooldown")
	c.InputDirs = ctx.StringSlice("input")
}

func (c *Input) Validate() error {
	if len(c.InputDirs) == 0 {
		return errors.New("at least one --input or CUESIX_INPUT_DIRS is required")
	}
	return nil
}
