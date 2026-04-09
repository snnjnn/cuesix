package config

import (
	"errors"
	"time"

	"github.com/urfave/cli/v3"
)

type Certmagic struct {
	Enabled         bool
	DefaultProvider string
	DataDir         string
	ChallengePort   int
	Timeout         time.Duration
	WatchInterval   time.Duration
	UntrackedGrace  time.Duration
	CleanupInterval time.Duration
	ExpiredGrace    time.Duration
	Providers       []string
}

// Flags returns Certmagic-related command-line flags.
func (c *Certmagic) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:     "certmagic",
			Usage:    "enable certmagic acme manager",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC"),
			Category: "Certmagic",
		},
		&cli.StringFlag{
			Name:     "certmagic-default-provider",
			Usage:    "certmagic default provider",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_DEFAULT_PROVIDER"),
			Category: "Certmagic",
		},
		&cli.StringFlag{
			Name:     "certmagic-data-dir",
			Usage:    "certmagic data directory",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_DATA_DIR"),
			Category: "Certmagic",
		},
		&cli.IntFlag{
			Name:     "certmagic-challenge-port",
			Usage:    "certmagic HTTP-01 challenge port",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_CHALLENGE_PORT"),
			Value:    8080,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-timeout",
			Usage:    "certmagic default certificate obtain timeout",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_TIMEOUT"),
			Value:    0,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-watch-interval",
			Usage:    "certmagic certificate refresh interval",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_WATCH_INTERVAL"),
			Value:    time.Hour,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-untracked-grace",
			Usage:    "grace period for removing untracked certmagic entries",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_UNTRACKED_GRACE"),
			Value:    7 * 24 * time.Hour,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-expired-interval",
			Usage:    "interval for removing expired certmagic entries",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_EXPIRED_INTERVAL"),
			Value:    24 * time.Hour,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-expired-grace",
			Usage:    "grace period for expired certmagic entries",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_EXPIRED_GRACE"),
			Value:    125 * time.Hour,
			Category: "Certmagic",
		},
		&cli.StringSliceFlag{
			Name:     "certmagic-provider",
			Usage:    "certmagic provider config (repeatable, format name|email|ca)",
			Sources:  cli.EnvVars("SIXPACK_CERTMAGIC_PROVIDERS"),
			Category: "Certmagic",
		},
	}
}

// Apply loads Certmagic settings from parsed CLI flags.
func (c *Certmagic) Apply(ctx *cli.Command) {
	c.Enabled = ctx.Bool("certmagic")
	c.DefaultProvider = ctx.String("certmagic-default-provider")
	c.DataDir = ctx.String("certmagic-data-dir")
	c.ChallengePort = ctx.Int("certmagic-challenge-port")
	c.Timeout = ctx.Duration("certmagic-timeout")
	c.WatchInterval = ctx.Duration("certmagic-watch-interval")
	c.UntrackedGrace = ctx.Duration("certmagic-untracked-grace")
	c.CleanupInterval = ctx.Duration("certmagic-expired-interval")
	c.ExpiredGrace = ctx.Duration("certmagic-expired-grace")
	c.Providers = ctx.StringSlice("certmagic-provider")
}

// Validate checks Certmagic configuration constraints.
func (c *Certmagic) Validate() error {
	if c.Enabled && c.WatchInterval <= 0 {
		return errors.New("certmagic watch interval must be positive")
	}
	if c.Enabled && c.DataDir == "" {
		return errors.New("certmagic data directory must be set")
	}
	return nil
}
