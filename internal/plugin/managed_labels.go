package plugin

import (
	"fmt"
	"log/slog"

	"github.com/warpcomdev/cuesix/internal/compiler"
)

const (
	ManagedByLabelKey   = "managed-by"
	ManagedByLabelValue = "sixpack"
	SixpackLabelKey     = "sixpack-label"
)

// ManagedLabelsPlugin injects ownership labels into APISIX resources that
// support labels, preserving any user-defined labels already present.
type ManagedLabelsPlugin struct {
	VirtualGateway string
	Rules          compiler.MergingRule
}

// Update adds sixpack ownership labels to top-level APISIX resources that
// support labels according to the compiler merge rules.
func (p *ManagedLabelsPlugin) Update(_ *slog.Logger, value map[string]any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	rules := p.Rules
	if len(rules.Children) == 0 {
		rules = compiler.DefaultMergingRules()
	}
	for key, rule := range rules.Children {
		if !rule.SupportsLabels {
			continue
		}
		rawItems, ok := value[key]
		if !ok {
			continue
		}
		items, ok := rawItems.([]any)
		if !ok {
			continue
		}
		for i, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			if err := ensureManagedLabels(item, p.VirtualGateway); err != nil {
				return nil, fmt.Errorf("resource %s[%d]: %w", key, i, err)
			}
		}
	}
	return value, nil
}

func ensureManagedLabels(item map[string]any, virtualgw string) error {
	rawLabels, ok := item["labels"]
	if !ok || rawLabels == nil {
		item["labels"] = map[string]any{
			ManagedByLabelKey: ManagedByLabelValue,
			SixpackLabelKey:   virtualgw,
		}
		return nil
	}
	labels, ok := rawLabels.(map[string]any)
	if !ok {
		return fmt.Errorf("labels must be an object, got %T", rawLabels)
	}
	labels[ManagedByLabelKey] = ManagedByLabelValue
	labels[SixpackLabelKey] = virtualgw
	return nil
}
