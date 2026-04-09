package config

import (
	"time"

	"github.com/urfave/cli/v3"
)

// Settings for any http server
type HTTPServer struct {
	ListenAddr        string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Settings specifically for the server mode
type Server struct {
	MetricsAddr string
	AutoTrigger bool
}

type Timeouts struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// Flags returns HTTP server and metrics command-line flags.
func (c *HTTPServer) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "listen",
			Usage:    "listen address",
			Sources:  cli.EnvVars("SIXPACK_LISTEN"),
			Value:    "127.0.0.1:8080",
			Category: "Server",
		},
		&cli.DurationFlag{
			Name:     "server-read-header-timeout",
			Usage:    "http server read header timeout",
			Sources:  cli.EnvVars("SIXPACK_SERVER_READ_HEADER_TIMEOUT"),
			Value:    5 * time.Second,
			Category: "Server",
		},
		&cli.DurationFlag{
			Name:     "server-read-timeout",
			Usage:    "http server read timeout",
			Sources:  cli.EnvVars("SIXPACK_SERVER_READ_TIMEOUT"),
			Value:    10 * time.Second,
			Category: "Server",
		},
		&cli.DurationFlag{
			Name:     "server-write-timeout",
			Usage:    "http server write timeout",
			Sources:  cli.EnvVars("SIXPACK_SERVER_WRITE_TIMEOUT"),
			Value:    10 * time.Second,
			Category: "Server",
		},
		&cli.DurationFlag{
			Name:     "server-idle-timeout",
			Usage:    "http server idle timeout",
			Sources:  cli.EnvVars("SIXPACK_SERVER_IDLE_TIMEOUT"),
			Value:    60 * time.Second,
			Category: "Server",
		},
		&cli.DurationFlag{
			Name:     "server-shutdown-timeout",
			Usage:    "http server shutdown timeout",
			Sources:  cli.EnvVars("SIXPACK_SERVER_SHUTDOWN_TIMEOUT"),
			Value:    10 * time.Second,
			Category: "Server",
		},
	}
}

// Flags returns HTTP server and metrics command-line flags.
func (c *Server) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "metrics",
			Usage:    "metrics listen address (empty to disable)",
			Sources:  cli.EnvVars("SIXPACK_METRICS_LISTEN"),
			Value:    ":8081",
			Category: "Server",
		},
		&cli.BoolFlag{
			Name:     "auto-trigger",
			Usage:    "trigger the compile loop automatically, once",
			Sources:  cli.EnvVars("SIXPACK_SERVER_AUTO_TRIGGER"),
			Value:    false,
			Category: "Server",
		},
	}
}

// Apply loads server settings from parsed CLI flags.
func (c *HTTPServer) Apply(ctx *cli.Command) {
	c.ListenAddr = ctx.String("listen")
	c.ReadHeaderTimeout = ctx.Duration("server-read-header-timeout")
	c.ReadTimeout = ctx.Duration("server-read-timeout")
	c.WriteTimeout = ctx.Duration("server-write-timeout")
	c.IdleTimeout = ctx.Duration("server-idle-timeout")
	c.ShutdownTimeout = ctx.Duration("server-shutdown-timeout")
}

// Apply loads server settings from parsed CLI flags.
func (c *Server) Apply(ctx *cli.Command) {
	c.MetricsAddr = ctx.String("metrics")
	c.AutoTrigger = ctx.Bool("auto-trigger")
}

// Validate input flags values
func (c *HTTPServer) Validate() error {
	return nil
}

// Validate input flags values
func (c *Server) Validate() error {
	return nil
}

// Timeouts returns the configured timeouts.
func (c *HTTPServer) Timeouts() Timeouts {
	return Timeouts{
		ReadHeaderTimeout: c.ReadHeaderTimeout,
		ReadTimeout:       c.ReadTimeout,
		WriteTimeout:      c.WriteTimeout,
		IdleTimeout:       c.IdleTimeout,
	}
}
