package config

import (
	"errors"
	"flag"
	"time"
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

func (c *Certmagic) RegisterFlags(fs *flag.FlagSet) {
	c.Providers = splitSemicolon(envString("CUESIX_CERTMAGIC_PROVIDERS"))
	fs.BoolVar(&c.Enabled, "certmagic", envBool("CUESIX_CERTMAGIC", false), "enable certmagic acme manager")
	fs.StringVar(&c.DefaultProvider, "certmagic-default-provider", envString("CUESIX_CERTMAGIC_DEFAULT_PROVIDER"), "certmagic default provider")
	fs.StringVar(&c.DataDir, "certmagic-data-dir", envString("CUESIX_CERTMAGIC_DATA_DIR"), "certmagic data directory")
	fs.StringVar(&c.ChallengeAddr, "certmagic-challenge-addr", envString("CUESIX_CERTMAGIC_CHALLENGE_ADDR"), "certmagic HTTP-01 challenge address")
	fs.DurationVar(&c.Timeout, "certmagic-timeout", envDuration("CUESIX_CERTMAGIC_TIMEOUT", 0), "certmagic default certificate obtain timeout")
	fs.DurationVar(&c.WatchInterval, "certmagic-watch-interval", envDuration("CUESIX_CERTMAGIC_WATCH_INTERVAL", time.Hour), "certmagic certificate refresh interval")
	fs.DurationVar(&c.UntrackedGrace, "certmagic-untracked-grace", envDuration("CUESIX_CERTMAGIC_UNTRACKED_GRACE", 7*24*time.Hour), "grace period for removing untracked certmagic entries")
	fs.DurationVar(&c.CleanupInterval, "certmagic-cleanup-interval", envDuration("CUESIX_CERTMAGIC_CLEANUP_INTERVAL", 24*time.Hour), "interval for removing expired certmagic entries")
	fs.DurationVar(&c.ExpiredGrace, "certmagic-expired-grace", envDuration("CUESIX_CERTMAGIC_EXPIRED_GRACE", 125*time.Hour), "grace period for expired certmagic entries")
	fs.Var(&stringSliceValue{target: &c.Providers}, "certmagic-provider", "certmagic provider config (repeatable)")
}

func (c *Certmagic) Validate() error {
	if c.Enabled && c.WatchInterval <= 0 {
		return errors.New("certmagic watch interval must be positive")
	}
	return nil
}
