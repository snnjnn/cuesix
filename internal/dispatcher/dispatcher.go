package dispatcher

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"slices"
	"time"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/cursor"
)

type Scheduler func(ctx context.Context, action func())

type Fetcher interface {
	Fetch() iter.Seq2[compiler.Snippet, error]
}

type State interface {
	// Reset the state to begin a new processing cycle
	Reset()
	// Commit the state after deploying to apisix succeeded
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
	Apply(ctx context.Context, virtualgw string, payload []byte) error
}

type ValidatorFactory interface {
	Instance(virtualgw string) Validator
}

type MergerFactory interface {
	Instance(virtualgw string) Merger
}

type SerializerFactory interface {
	Instance(virtualgw string) Serializer
}

// Config wires the dispatcher dependencies and runtime options.
type Config struct {
	// Compiler, Cache, Validator, and Reloader define the pipeline stages.
	Fetcher           Fetcher
	MergerFactory     MergerFactory
	SerializerFactory SerializerFactory
	ValidatorFactory  ValidatorFactory
	Reloader          Reloader
	Scheduler         Scheduler
	// Filesystems provide the input directories to read YAML fragments from.
	Filesystems compiler.Input
	// OutputYAML controls whether validation is performed against YAML instead of JSON.
	OutputYAML bool
	// Cooldown defines the minimum interval between dequeued runs.
	Cooldown time.Duration
}

type VirtualGateway struct {
	merger        Merger
	serializer    Serializer
	validator     Validator
	lastCommit    []byte
	commitSuccess bool
}

// Dispatcher queues compile requests and runs the compile/validate/reload pipeline.
type Dispatcher struct {
	// Logger for this dispatcher instance
	logger       *slog.Logger
	config       Config
	queue        chan struct{}
	lastDequeued time.Time
	// Last running success
	virtualgw map[string]*VirtualGateway
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
	if cfg.Filesystems == nil {
		return nil, errors.New("filesystems are required")
	}
	if len(cfg.Filesystems.Namespaces()) == 0 {
		return nil, errors.New("filesystems are required")
	}
	gateways := make(map[string]*VirtualGateway)
	return &Dispatcher{
		logger:    logger,
		config:    cfg,
		queue:     make(chan struct{}, 1),
		virtualgw: gateways,
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
func (d *Dispatcher) Run(ctx context.Context) error {
	if d == nil {
		return errors.New("dispatcher is nil")
	}
	logger := d.logger
	if err := d.waitForCooldown(ctx); err != nil {
		return err
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
	return ctx.Err()
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

	stageStart := time.Now()
	// TODO: Add snippets by label, and validate together?
	logger.Info("fetch stage start")
	snippets := make(map[string][]compiler.Snippet)
	for snippet, err := range d.config.Fetcher.Fetch() {
		if err != nil {
			dispatcherErrors.WithLabelValues("fetch").Inc()
			return err
		}
		if snippet.Data == nil {
			logger.Warn("skipping empty snippet (decoded nil map)", "source", snippet.Ref.Key())
			continue
		}
		// Support hierarchical virtual gateways (e.g. "foo/bar")
		for _, virtualgw := range snippet.Virtualgw.Hierarchy() {
			snippets[virtualgw] = append(snippets[virtualgw], snippet)
		}
	}
	if len(snippets) == 0 {
		logger.Warn("no snippets fetched after filtering")
	}
	dispatcherDuration.WithLabelValues("fetch").Observe(time.Since(stageStart).Seconds())
	logger.Info("fetch stage complete", "duration", time.Since(stageStart))

	// Vag processing
	errs := make([]error, 0, len(snippets))
	for virtualgw, snippetList := range snippets {
		state, ok := d.virtualgw[virtualgw]
		if !ok {
			state = &VirtualGateway{
				merger:     d.config.MergerFactory.Instance(virtualgw),
				serializer: d.config.SerializerFactory.Instance(virtualgw),
				validator:  d.config.ValidatorFactory.Instance(virtualgw),
			}
			d.virtualgw[virtualgw] = state
		}
		if err := d.handleGateway(ctx, virtualgw, state, snippetList); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (d *Dispatcher) handleGateway(ctx context.Context, vgwname string, virtualgw *VirtualGateway, snippets []compiler.Snippet) error {
	// Reset the commitSuccess flag
	wasSucessful := virtualgw.commitSuccess
	virtualgw.commitSuccess = false

	logger := d.logger.With("virtualgw", vgwname)
	stageStart := time.Now()
	logger.Info("Merge stage start")

	virtualgw.merger.Reset()
	merged, err := virtualgw.merger.Merge(slices.Values(snippets))
	if err != nil {
		dispatcherErrors.WithLabelValues("merge").Inc()
		return err
	}
	dispatcherDuration.WithLabelValues("merge").Observe(time.Since(stageStart).Seconds())
	logger.Info("merge stage complete", "duration", time.Since(stageStart))

	stageStart = time.Now()
	logger.Info("serialization stage start")
	virtualgw.serializer.Reset()
	serialized, err := virtualgw.serializer.Serialize(merged)
	if err != nil {
		dispatcherErrors.WithLabelValues("serialize").Inc()
		return err
	}
	dispatcherDuration.WithLabelValues("serialize").Observe(time.Since(stageStart).Seconds())
	// No changes detected since last committed config
	if serialized == nil {
		if wasSucessful {
			logger.Info("no changes detected; skipping reload")
			virtualgw.commitSuccess = true
			return nil
		}
		logger.Info("will retry last committed config")
		serialized = virtualgw.lastCommit
	}
	logger.Info("serialize stage complete", "duration", time.Since(stageStart))

	stageStart = time.Now()
	logger.Info("validation stage start")
	virtualgw.validator.Reset()
	ok, err := virtualgw.validator.Validate(serialized, d.config.OutputYAML)
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
	if err := d.config.Reloader.Apply(ctx, vgwname, serialized); err != nil {
		dispatcherDuration.WithLabelValues("reload").Observe(time.Since(stageStart).Seconds())
		dispatcherErrors.WithLabelValues("reload").Inc()
		return err
	}
	dispatcherDuration.WithLabelValues("reload").Observe(time.Since(stageStart).Seconds())
	logger.Info("reload stage complete", "duration", time.Since(stageStart))

	// Register configuration as successful
	virtualgw.lastCommit = serialized
	virtualgw.commitSuccess = true

	// And commit state
	virtualgw.merger.Commit()
	virtualgw.serializer.Commit()
	virtualgw.validator.Commit()
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
