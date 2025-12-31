package config

import (
	"flag"
	"time"
)

type APISIX struct {
	Home              string
	MirrorDir         string
	KeepMirror        bool
	ValidationTimeout time.Duration
}

func (c *APISIX) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.Home, "apisix-home", envStringDefault("CUESIX_APISIX_HOME", "/usr/local/apisix"), "apisix home path")
	fs.StringVar(&c.MirrorDir, "mirror-dir", envString("CUESIX_MIRROR_DIR"), "apisix mirror directory (optional)")
	fs.BoolVar(&c.KeepMirror, "keep-mirror", envBool("CUESIX_KEEP_MIRROR", false), "Do not remove mirror on startup")
	fs.DurationVar(&c.ValidationTimeout, "validation-timeout", envDuration("CUESIX_VALIDATION_TIMEOUT", 30*time.Second), "timeout for apisix test")
}
