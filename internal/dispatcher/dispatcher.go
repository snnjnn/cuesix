package dispatcher

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"log/slog"
	"slices"
	"time"

	"github.com/warpcomdev/sixpack/internal/compiler"
	"github.com/warpcomdev/sixpack/internal/cursor"
)

type Scheduler func(ctx context.Context, action func())

type Fetcher interface {
	Fetch(fses ...fs.FS) iter.Seq2[compiler.Snippet, error]
}

type State interface {
	// Reset the state to begin a new processing cycle
	Reset()
	// Commit the stae after deploying to apisix succeeded
	Commit()
}

type Merger interface {
	State
	Merge(snippets iter.Seq[compiler.Snippet]) (map[string]any, error)
}

type Serializer interface {
	State
	Serialize(value map[string]any) ([]byte, error)
}

type Validator interface {
	State
	Validate(candidate []byte, isYAML bool) (bool, error)
}

type Reloader interface {
	Apply(ctx context.Context, payload []byte, useApi bool) error
}

// Config wires the dispatcher dependencies and runtime options.
type Config struct {
	// Compiler, Cache, Validator, and Reloader define the pipeline stages.
	Fetcher           Fetcher
	MergerFactory     func() Merger
	SerializerFactory func() Serializer
	ValidatorFactory  func() Validator
	Reloader          Reloader
	Scheduler         Scheduler
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
	// Last running success
	lastCommit    []byte
	commitSuccess bool
	// Virtual API gateways state
	vag state
	// Logger for this dispatcher instance
	logger *slog.Logger
}

// New builds a dispatcher with the provided configuration.
func New(logger *slog.Logger, cfg Config) (*Dispatcher, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Fetcher == nil {
		return nil, errors.New("fetcher is required")
	}
	if cfg.MergerFactory == nil {
		return nil, errors.New("merger is required")
	}
	if cfg.SerializerFactory == nil {
		return nil, errors.New("serializer is required")
	}
	if cfg.ValidatorFactory == nil {
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
		logger: logger,
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

type state struct {
	merger     Merger
	serializer Serializer
	validator  Validator
}

// Run consumes queued events until the context is canceled.
func (d *Dispatcher) Run(ctx context.Context) error {
	if d == nil {
		return errors.New("dispatcher is nil")
	}
	logger := d.logger
	if err := d.waitForCooldown(ctx); err != nil {
		return err
	}
	d.vag = state{
		merger:     d.config.MergerFactory(),
		serializer: d.config.SerializerFactory(),
		validator:  d.config.ValidatorFactory(),
	}
	for range cursor.All(ctx, cursor.Channel(d.queue)) {
		dispatcherDequeued.Inc()
		dequeuedAt := time.Now()
		if err := d.waitForCooldown(ctx); err != nil {
			return err
		}
		d.lastDequeued = dequeuedAt
		logger.Info("compile request dequeued", "cooldown", d.config.Cooldown)
		d.config.Scheduler(ctx, func() {
			if err := d.handle(ctx); err != nil {
				logger.Error("compile pipeline failed", "error", err)
			}
		})
	}
	return nil
}

// Internal function that models the pipeline
func (d *Dispatcher) handle(ctx context.Context) error {
	if d == nil {
		return errors.New("dispatcher is nil")
	}
	logger := d.logger
	start := time.Now()
	defer func() {
		dispatcherDuration.WithLabelValues("total").Observe(time.Since(start).Seconds())
	}()

	// Reset the commitSuccess flag
	wasSucessful := d.commitSuccess
	d.commitSuccess = false

	stageStart := time.Now()
	// TODO: Add snippets by label, and validate together?
	logger.Info("fetch stage start")
	snippets := make([]compiler.Snippet, 0, 10)
	for snippet, err := range d.config.Fetcher.Fetch(d.config.Filesystems...) {
		if err != nil {
			dispatcherErrors.WithLabelValues("fetch").Inc()
			return err
		}
		snippets = append(snippets, snippet)
	}
	dispatcherDuration.WithLabelValues("fetch").Observe(time.Since(stageStart).Seconds())
	logger.Info("fetch stage complete", "duration", time.Since(stageStart))

	// VAG selection: currently we only support one VAG
	vag := d.vag

	stageStart = time.Now()
	logger.Info("Merge stage start")
	vag.merger.Reset()
	merged, err := vag.merger.Merge(slices.Values(snippets))
	if err != nil {
		dispatcherErrors.WithLabelValues("merge").Inc()
		return err
	}
	dispatcherDuration.WithLabelValues("merge").Observe(time.Since(stageStart).Seconds())
	logger.Info("merge stage complete", "duration", time.Since(stageStart))

	stageStart = time.Now()
	logger.Info("serialization stage start")
	vag.serializer.Reset()
	serialized, err := vag.serializer.Serialize(merged)
	if err != nil {
		dispatcherErrors.WithLabelValues("serialize").Inc()
		return err
	}
	dispatcherDuration.WithLabelValues("serialize").Observe(time.Since(stageStart).Seconds())
	// No changes detected since last committed config
	if serialized == nil {
		if wasSucessful {
			logger.Info("no changes detected; skipping reload")
			d.commitSuccess = true
			return nil
		}
		logger.Info("will retry last committed config")
		serialized = d.lastCommit
	}
	logger.Info("serialize stage complete", "duration", time.Since(stageStart))

	stageStart = time.Now()
	logger.Info("validation stage start")
	vag.validator.Reset()
	ok, err := vag.validator.Validate(serialized, d.config.OutputYAML)
	if err != nil {
		dispatcherErrors.WithLabelValues("validate").Inc()
		return err
	}
	if !ok {
		logger.Warn("validation failed")
		dispatcherErrors.WithLabelValues("validate").Inc()
		return errors.New("validation failed")
	}
	dispatcherDuration.WithLabelValues("validate").Observe(time.Since(stageStart).Seconds())
	logger.Info("validation stage complete", "duration", time.Since(stageStart))

	stageStart = time.Now()
	logger.Info("reload stage start")
	if err := d.config.Reloader.Apply(ctx, serialized, true); err != nil {
		dispatcherDuration.WithLabelValues("reload").Observe(time.Since(stageStart).Seconds())
		dispatcherErrors.WithLabelValues("reload").Inc()
		return err
	}
	dispatcherDuration.WithLabelValues("reload").Observe(time.Since(stageStart).Seconds())
	logger.Info("reload stage complete", "duration", time.Since(stageStart))

	// Register configuration as successful
	d.lastCommit = serialized
	d.commitSuccess = true

	// And commit state
	vag.merger.Commit()
	vag.serializer.Commit()
	vag.validator.Commit()
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
