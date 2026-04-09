package plugin

import "log/slog"

// PreRender modifies the merged config map before serialization.
type PreRender interface {
	Update(logger *slog.Logger, value map[string]any) (map[string]any, error)
}

// PostRender modifies the serialized payload.
type PostRender interface {
	Update(logger *slog.Logger, value []byte) ([]byte, error)
}

// PreRenderChain applies multiple PreRender plugins in order.
type PreRenderChain []PreRender

// Update runs each pre-render plugin in order, feeding output to the next.
func (c PreRenderChain) Update(logger *slog.Logger, value map[string]any) (map[string]any, error) {
	current := value
	for _, p := range c {
		if p == nil {
			continue
		}
		next, err := p.Update(logger, current)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

// PostRenderChain applies multiple PostRender plugins in order.
type PostRenderChain []PostRender

// Update runs each post-render plugin in order, feeding output to the next.
func (c PostRenderChain) Update(logger *slog.Logger, value []byte) ([]byte, error) {
	current := value
	for _, p := range c {
		if p == nil {
			continue
		}
		next, err := p.Update(logger, current)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

// PostRenderFunc adapts a function to a PostRender plugin.
type PostRenderFunc func(logger *slog.Logger, value []byte) ([]byte, error)

// Update calls the wrapped post-render function.
func (p PostRenderFunc) Update(logger *slog.Logger, value []byte) ([]byte, error) {
	return p(logger, value)
}

// PreRenderFunc adapts a function to a PreRender plugin.
type PreRenderFunc func(logger *slog.Logger, value map[string]any) (map[string]any, error)

// Update calls the wrapped pre-render function.
func (p PreRenderFunc) Update(logger *slog.Logger, value map[string]any) (map[string]any, error) {
	return p(logger, value)
}
