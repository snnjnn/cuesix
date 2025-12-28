package dispatcher

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"time"
)

type Compiler interface {
	Compile(fses ...fs.FS) (map[string]any, error)
}

type Cache interface {
	Changed(value map[string]any) ([]byte, error)
}

type Validator interface {
	Validate(candidate []byte, isYAML bool) (bool, error)
}

type Reloader interface {
	Apply(ctx context.Context, payload []byte) error
}

// Config wires the dispatcher dependencies and runtime options.
type Config struct {
	// Compiler, Cache, Validator, and Reloader define the pipeline stages.
	Compiler  Compiler
	Cache     Cache
	Validator Validator
	Reloader  Reloader
	// Filesystems provide the input directories to read YAML fragments from.
	Filesystems []fs.FS
	// OutputYAML controls whether validation is performed against YAML instead of JSON.
	OutputYAML bool
	// Cooldown defines the minimum interval between dequeued runs.
	Cooldown time.Duration
	Logger   *slog.Logger
}

// Dispatcher queues compile requests and runs the compile/validate/reload pipeline.
type Dispatcher struct {
	config       Config
	queue        chan struct{}
	lastDequeued time.Time
}

// New builds a dispatcher with the provided configuration.
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
	// cfg is passed by value, so there is no risk
	// overwriting the Logger pointer field.
	cfg.Logger = ensureLogger(cfg.Logger)
	return &Dispatcher{
		config: cfg,
		queue:  make(chan struct{}, 1),
	}, nil
}

// Notify enqueues a compile request if the queue is empty.
func (d *Dispatcher) Notify() {
	select {
	case d.queue <- struct{}{}:
	default:
	}
}

// Run consumes queued events until the context is canceled.
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
			d.config.Logger.Info("compile request dequeued", "cooldown", d.config.Cooldown)
			if err := d.handle(ctx); err != nil {
				d.config.Logger.Error("compile pipeline failed", "error", err)
				return err
			}
		}
	}
}

func (d *Dispatcher) handle(ctx context.Context) error {
	merged, err := d.config.Compiler.Compile(d.config.Filesystems...)
	if err != nil {
		return err
	}

	normalized, err := d.config.Cache.Changed(merged)
	if err != nil {
		return err
	}
	if normalized == nil {
		d.config.Logger.Info("no changes detected; skipping validation and reload")
		return nil
	}

	ok, err := d.config.Validator.Validate(normalized, d.config.OutputYAML)
	if err != nil {
		return err
	}
	if !ok {
		d.config.Logger.Warn("validation failed")
		return errors.New("validation failed")
	}

	if err := d.config.Reloader.Apply(ctx, normalized); err != nil {
		return err
	}
	d.config.Logger.Info("reload completed")
	return nil
}

func (d *Dispatcher) waitForCooldown(ctx context.Context) error {
	if d.config.Cooldown <= 0 || d.lastDequeued.IsZero() {
		return nil
	}
	remaining := d.config.Cooldown - time.Since(d.lastDequeued)
	if remaining <= 0 {
		return nil
	}
	return sleepContext(ctx, remaining)
}

func (d *Dispatcher) waitAfterDequeue(ctx context.Context, dequeuedAt time.Time) error {
	if d.config.Cooldown <= 0 || d.lastDequeued.IsZero() {
		return nil
	}
	remaining := d.config.Cooldown - dequeuedAt.Sub(d.lastDequeued)
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
