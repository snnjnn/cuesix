package config

import (
	"errors"
	"time"

	"github.com/urfave/cli/v2"
)

type Certmagic struct {
	Enabled         bool
	DefaultProvider string
	DataDir         string
	ChallengeAddr   string
	Timeout         time.Duration
	WatchInterval   time.Duration
	UntrackedGrace  time.Duration
	CleanupInterval time.Duration
	ExpiredGrace    time.Duration
	Providers       []string
}

func (c *Certmagic) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:     "certmagic",
			Usage:    "enable certmagic acme manager",
			EnvVars:  []string{"CUESIX_CERTMAGIC"},
			Category: "Certmagic",
		},
		&cli.StringFlag{
			Name:     "certmagic-default-provider",
			Usage:    "certmagic default provider",
			EnvVars:  []string{"CUESIX_CERTMAGIC_DEFAULT_PROVIDER"},
			Category: "Certmagic",
		},
		&cli.StringFlag{
			Name:     "certmagic-data-dir",
			Usage:    "certmagic data directory",
			EnvVars:  []string{"CUESIX_CERTMAGIC_DATA_DIR"},
			Category: "Certmagic",
		},
		&cli.StringFlag{
			Name:     "certmagic-challenge-addr",
			Usage:    "certmagic HTTP-01 challenge address",
			EnvVars:  []string{"CUESIX_CERTMAGIC_CHALLENGE_ADDR"},
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-timeout",
			Usage:    "certmagic default certificate obtain timeout",
			EnvVars:  []string{"CUESIX_CERTMAGIC_TIMEOUT"},
			Value:    0,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-watch-interval",
			Usage:    "certmagic certificate refresh interval",
			EnvVars:  []string{"CUESIX_CERTMAGIC_WATCH_INTERVAL"},
			Value:    time.Hour,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-untracked-grace",
			Usage:    "grace period for removing untracked certmagic entries",
			EnvVars:  []string{"CUESIX_CERTMAGIC_UNTRACKED_GRACE"},
			Value:    7 * 24 * time.Hour,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-cleanup-interval",
			Usage:    "interval for removing expired certmagic entries",
			EnvVars:  []string{"CUESIX_CERTMAGIC_CLEANUP_INTERVAL"},
			Value:    24 * time.Hour,
			Category: "Certmagic",
		},
		&cli.DurationFlag{
			Name:     "certmagic-expired-grace",
			Usage:    "grace period for expired certmagic entries",
			EnvVars:  []string{"CUESIX_CERTMAGIC_EXPIRED_GRACE"},
			Value:    125 * time.Hour,
			Category: "Certmagic",
		},
		&cli.StringSliceFlag{
			Name:     "certmagic-provider",
			Usage:    "certmagic provider config (repeatable, format name|email|ca)",
			EnvVars:  []string{"CUESIX_CERTMAGIC_PROVIDERS"},
			Category: "Certmagic",
		},
	}
}

func (c *Certmagic) Apply(ctx *cli.Context) {
	c.Enabled = ctx.Bool("certmagic")
	c.DefaultProvider = ctx.String("certmagic-default-provider")
	c.DataDir = ctx.String("certmagic-data-dir")
	c.ChallengeAddr = ctx.String("certmagic-challenge-addr")
	c.Timeout = ctx.Duration("certmagic-timeout")
	c.WatchInterval = ctx.Duration("certmagic-watch-interval")
	c.UntrackedGrace = ctx.Duration("certmagic-untracked-grace")
	c.CleanupInterval = ctx.Duration("certmagic-cleanup-interval")
	c.ExpiredGrace = ctx.Duration("certmagic-expired-grace")
	c.Providers = ctx.StringSlice("certmagic-provider")
}

func (c *Certmagic) Validate() error {
	if c.Enabled && c.WatchInterval <= 0 {
		return errors.New("certmagic watch interval must be positive")
	}
	return nil
}
