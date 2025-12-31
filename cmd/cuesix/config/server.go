package config

import (
	"flag"
	"time"
)

type Server struct {
	ListenAddr        string
	MetricsAddr       string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

func (c *Server) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.ListenAddr, "listen", envStringDefault("CUESIX_LISTEN", "127.0.0.1:8080"), "listen address")
	fs.StringVar(&c.MetricsAddr, "metrics", envStringDefault("CUESIX_METRICS_LISTEN", ":8081"), "metrics listen address (empty to disable)")
	fs.DurationVar(&c.ReadHeaderTimeout, "server-read-header-timeout", envDuration("CUESIX_SERVER_READ_HEADER_TIMEOUT", 5*time.Second), "http server read header timeout")
	fs.DurationVar(&c.ReadTimeout, "server-read-timeout", envDuration("CUESIX_SERVER_READ_TIMEOUT", 10*time.Second), "http server read timeout")
	fs.DurationVar(&c.WriteTimeout, "server-write-timeout", envDuration("CUESIX_SERVER_WRITE_TIMEOUT", 10*time.Second), "http server write timeout")
	fs.DurationVar(&c.IdleTimeout, "server-idle-timeout", envDuration("CUESIX_SERVER_IDLE_TIMEOUT", 60*time.Second), "http server idle timeout")
	fs.DurationVar(&c.ShutdownTimeout, "server-shutdown-timeout", envDuration("CUESIX_SERVER_SHUTDOWN_TIMEOUT", 10*time.Second), "http server shutdown timeout")
}
