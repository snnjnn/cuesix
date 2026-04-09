package compiler

type Snippet struct {
	Ref       SourceRef
	Virtualgw VirtualGateway
	Data      map[string]any
}

// AsTree converts a snippet to a tree of objects by kind and id
func (root MergingRule) AsTree(s Snippet) map[string]map[string]map[string]any {
	ids := make(map[string]map[string]map[string]any)
	for key, val := range s.Data {
		if child, ok := root.Children[key]; ok {
			if child.Kind == KindList {
				if itemList, ok := val.([]any); ok {
					nested := make(map[string]map[string]any)
					for _, item := range itemList {
						if itemMap, ok := item.(map[string]any); ok {
							if id, ok := itemMap[child.IDAttr]; ok {
								if idString, ok := asString(id); ok {
									nested[idString] = itemMap
								}
							}
						}
					}
					ids[key] = nested
				}
			}
			if child.Kind == KindSet {
				if itemMap, ok := val.(map[string]any); ok {
					nested := make(map[string]map[string]any)
					for id, item := range itemMap {
						if entry, ok := item.(map[string]any); ok {
							nested[id] = entry
						}
					}
					ids[key] = nested
				}
			}
		}
	}
	return ids
}
