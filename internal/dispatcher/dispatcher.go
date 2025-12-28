package dispatcher

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"time"
)

type Compiler interface {
	Compile(logger *slog.Logger, fses ...fs.FS) (map[string]any, error)
}

type Cache interface {
	Changed(logger *slog.Logger, value map[string]any) ([]byte, error)
}

type Validator interface {
	Validate(logger *slog.Logger, candidate []byte, isYAML bool) (bool, error)
}

type Reloader interface {
	Apply(ctx context.Context, logger *slog.Logger, payload []byte) error
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
	return &Dispatcher{
		config: cfg,
		queue:  make(chan struct{}, 1),
	}, nil
}

// Notify enqueues a compile request if the queue is empty.
func (d *Dispatcher) Notify() {
	select {
	case d.queue <- struct{}{}:
		dispatcherEnqueued.Inc()
	default:
		dispatcherDropped.Inc()
	}
}

// Run consumes queued events until the context is canceled.
func (d *Dispatcher) Run(ctx context.Context, logger *slog.Logger) error {
	if err := d.waitForCooldown(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.queue:
			dispatcherDequeued.Inc()
			dequeuedAt := time.Now()
			if err := d.waitAfterDequeue(ctx, dequeuedAt); err != nil {
				return err
			}
			d.lastDequeued = dequeuedAt
			logger.Info("compile request dequeued", "cooldown", d.config.Cooldown)
			if err := d.handle(ctx, logger); err != nil {
				logger.Error("compile pipeline failed", "error", err)
				return err
			}
		}
	}
}

func (d *Dispatcher) handle(ctx context.Context, logger *slog.Logger) error {
	start := time.Now()
	defer dispatcherDuration.WithLabelValues("total").Observe(time.Since(start).Seconds())

	stageStart := time.Now()
	logger.Info("compile stage start")
	merged, err := d.config.Compiler.Compile(logger, d.config.Filesystems...)
	dispatcherDuration.WithLabelValues("compile").Observe(time.Since(stageStart).Seconds())
	if err != nil {
		dispatcherErrors.WithLabelValues("compile").Inc()
		return err
	}
	logger.Info("compile stage complete", "duration", time.Since(stageStart))

	stageStart = time.Now()
	logger.Info("cache stage start")
	normalized, err := d.config.Cache.Changed(logger, merged)
	dispatcherDuration.WithLabelValues("cache").Observe(time.Since(stageStart).Seconds())
	if err != nil {
		dispatcherErrors.WithLabelValues("cache").Inc()
		return err
	}
	if normalized == nil {
		dispatcherSkipped.Inc()
		logger.Info("no changes detected; skipping validation and reload")
		return nil
	}
	logger.Info("cache stage complete", "duration", time.Since(stageStart))

	stageStart = time.Now()
	logger.Info("validation stage start")
	ok, err := d.config.Validator.Validate(logger, normalized, d.config.OutputYAML)
	dispatcherDuration.WithLabelValues("validate").Observe(time.Since(stageStart).Seconds())
	if err != nil {
		dispatcherErrors.WithLabelValues("validate").Inc()
		return err
	}
	if !ok {
		logger.Warn("validation failed")
		dispatcherErrors.WithLabelValues("validate").Inc()
		return errors.New("validation failed")
	}
	logger.Info("validation stage complete", "duration", time.Since(stageStart))

	stageStart = time.Now()
	logger.Info("reload stage start")
	if err := d.config.Reloader.Apply(ctx, logger, normalized); err != nil {
		dispatcherDuration.WithLabelValues("reload").Observe(time.Since(stageStart).Seconds())
		dispatcherErrors.WithLabelValues("reload").Inc()
		return err
	}
	dispatcherDuration.WithLabelValues("reload").Observe(time.Since(stageStart).Seconds())
	logger.Info("reload stage complete", "duration", time.Since(stageStart))
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
