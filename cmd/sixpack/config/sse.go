package config

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v3"
)

type ServerSideEvents struct {
	KeepAlive time.Duration
}

// Flags returns APISIX runtime and validation command-line flags.
func (c *ServerSideEvents) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.DurationFlag{
			Name:     "plugin-sse-keepalive",
			Usage:    "Enable server-side-events endpoints and configure keepalive",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_SSE_KEEPALIVE"),
			Category: "SSE",
		},
	}
}

// Apply loads APISIX configuration values from parsed CLI flags.
func (c *ServerSideEvents) Apply(ctx *cli.Command) {
	c.KeepAlive = ctx.Duration("plugin-sse-keepalive")
}

// Validate verifies SSE settings.
func (c *ServerSideEvents) Validate() error {
	if c.KeepAlive < 0 {
		return fmt.Errorf("plugin-sse-keepalive must be >= 0")
	}
	return nil
}
