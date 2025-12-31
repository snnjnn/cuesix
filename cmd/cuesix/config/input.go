package config

import (
	"errors"
	"flag"
	"time"
)

type Input struct {
	Serve     bool
	InputDirs []string
	Cooldown  time.Duration
}

func (c *Input) RegisterFlags(fs *flag.FlagSet) {
	c.InputDirs = splitComma(envString("CUESIX_INPUT_DIRS"))
	fs.BoolVar(&c.Serve, "serve", envBool("CUESIX_SERVE", false), "run HTTP server")
	fs.DurationVar(&c.Cooldown, "cooldown", envDuration("CUESIX_COOLDOWN", 0), "cooldown duration")
	fs.Var(&stringSliceValue{target: &c.InputDirs}, "input", "input directory (repeatable)")
}

func (c *Input) Validate() error {
	if len(c.InputDirs) == 0 {
		return errors.New("at least one --input or CUESIX_INPUT_DIRS is required")
	}
	return nil
}
