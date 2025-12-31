package ssl

import (
	"log/slog"
)

type TextHandler struct{}

func (f TextHandler) replaceTargets(logger *slog.Logger, targets []certTargets, fallback Certificate) {
	// no op, se dejan como están
}
