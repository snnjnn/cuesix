package compiler

func routeTags(route map[string]any) map[string][]string {
	hosts := make([]string, 0, 2)
	paths := make([]string, 0, 2)
	if hostsItf, ok := route["hosts"]; ok {
		hosts = append(hosts, collectStrings(hostsItf)...)
	}
	if pathsItf, ok := route["uris"]; ok {
		paths = append(paths, collectStrings(pathsItf)...)
	}
	if host, ok := route["host"].(string); ok {
		hosts = append(hosts, host)
	}
	if path, ok := route["uri"].(string); ok {
		paths = append(paths, path)
	}
	result := make(map[string][]string)
	if len(hosts) > 0 {
		result["hosts"] = hosts
	}
	if len(paths) > 0 {
		result["uris"] = paths
	}
	return result
}

func collectStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		asString, ok := item.(string)
		if !ok {
			continue
		}
		out = append(out, asString)
	}
	return out
}
