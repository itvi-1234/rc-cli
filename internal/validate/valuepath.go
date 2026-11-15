package validate

import (
	"fmt"
	"slices"
	"strings"
)

type foundValue struct {
	path  string
	value any
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// valuesAt returns every value a condition holds at a field path such as interface.engine,
// interface.operations[].method or configuration.alternatives[].env[].property. A segment that ends
// in [] walks the elements of a list. Each value comes back with the path that reaches it, list
// indexes included, so a message can point at the element.
func valuesAt(root any, path string) []foundValue {
	found := []foundValue{{path: "", value: root}}
	for _, segment := range strings.Split(path, ".") {
		name, list := strings.CutSuffix(segment, "[]")
		var next []foundValue
		for _, current := range found {
			object, ok := current.value.(map[string]any)
			if !ok {
				continue
			}
			value, present := object[name]
			if !present {
				continue
			}
			here := current.path
			if here != "" {
				here += "."
			}
			here += name
			if !list {
				next = append(next, foundValue{path: here, value: value})
				continue
			}
			items, ok := value.([]any)
			if !ok {
				continue
			}
			for index, item := range items {
				next = append(next, foundValue{path: fmt.Sprintf("%s[%d]", here, index), value: item})
			}
		}
		found = next
	}
	return found
}
