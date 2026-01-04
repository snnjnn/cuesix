package ssl

import (
	"fmt"
	"path"
	"strings"
)

func sanitizePath(name string) (string, error) {
	clean := path.Clean(name)
	if clean == "." || clean == "/" || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid file path: %s", name)
	}
	return clean, nil
}
