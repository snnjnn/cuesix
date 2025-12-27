package dispatcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"log/slog"
	"time"
)

type Compiler interface {
	Compile(fses ...fs.FS) (map[string]any, error)
}

type Cache interface {
	Changed(value map[string]any) (string, error)
}

type Validator interface {
	Validate(configPath string) (bool, error)
}

type Reloader interface {
	Apply(ctx context.Context, tempPath string) error
}

type Config struct {
	Compiler   Compiler
	Cache      Cache
	Validator  Validator
	Reloader   Reloader
	Filesystems []fs.FS
	Cooldown    time.Duration
	Logger      *slog.Logger
}

type Dispatcher struct {
	queue       chan struct{}
	compiler    Compiler
	cache       Cache
	validator   Validator
	reloader    Reloader
	filesystems []fs.FS
	cooldown    time.Duration
	lastDequeued time.Time
	logger      *slog.Logger
}

func New(cfg Config) (*Dispatcher, error) {
	if cfg.Compiler == nil {
		return nil, errors.New("compiler is required")
	}
	if cfg.Cache == nil {
		return nil, errors.New("cache is required")
	}
	if cfg.Validator == nil {
		return nil, errors.New("validator is required")
	}
	if cfg.Reloader == nil {
		return nil, errors.New("reloader is required")
	}
	if len(cfg.Filesystems) == 0 {
		return nil, errors.New("filesystems are required")
	}
	return &Dispatcher{
		queue:       make(chan struct{}, 1),
		compiler:    cfg.Compiler,
		cache:       cfg.Cache,
		validator:   cfg.Validator,
		reloader:    cfg.Reloader,
		filesystems: cfg.Filesystems,
		cooldown:    cfg.Cooldown,
		logger:      ensureLogger(cfg.Logger),
	}, nil
}

func (d *Dispatcher) Notify() {
	select {
	case d.queue <- struct{}{}:
	default:
	}
}

func (d *Dispatcher) Run(ctx context.Context) error {
	if err := d.waitForCooldown(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.queue:
			dequeuedAt := time.Now()
			if err := d.waitAfterDequeue(ctx, dequeuedAt); err != nil {
				return err
			}
			d.lastDequeued = dequeuedAt
			d.logger.Info("compile request dequeued", "cooldown", d.cooldown)
			if err := d.handle(ctx); err != nil {
				d.logger.Error("compile pipeline failed", "error", err)
				return err
			}
		}
	}
}

func (d *Dispatcher) handle(ctx context.Context) error {
	merged, err := d.compiler.Compile(d.filesystems...)
	if err != nil {
		return err
	}

	tempPath, err := d.cache.Changed(merged)
	if err != nil {
		return err
	}
	if tempPath == "" {
		d.logger.Info("no changes detected; skipping validation and reload")
		return nil
	}
	defer func() {
		_ = os.Remove(tempPath)
	}()

	ok, err := d.validator.Validate(tempPath)
	if err != nil {
		return err
	}
	if !ok {
		d.logger.Warn("validation failed")
		return errors.New("validation failed")
	}

	if err := d.reloader.Apply(ctx, tempPath); err != nil {
		return err
	}
	d.logger.Info("reload completed")
	return nil
}

func (d *Dispatcher) waitForCooldown(ctx context.Context) error {
	if d.cooldown <= 0 || d.lastDequeued.IsZero() {
		return nil
	}
	remaining := d.cooldown - time.Since(d.lastDequeued)
	if remaining <= 0 {
		return nil
	}
	return sleepContext(ctx, remaining)
}

func (d *Dispatcher) waitAfterDequeue(ctx context.Context, dequeuedAt time.Time) error {
	if d.cooldown <= 0 || d.lastDequeued.IsZero() {
		return nil
	}
	remaining := d.cooldown - dequeuedAt.Sub(d.lastDequeued)
	if remaining <= 0 {
		return nil
	}
	return sleepContext(ctx, remaining)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func ensureLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}
