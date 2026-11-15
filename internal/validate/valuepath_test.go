package validate

import (
	"reflect"
	"testing"
)

func pathsOf(found []foundValue) []string {
	paths := []string{}
	for _, item := range found {
		paths = append(paths, item.path)
	}
	return paths
}

func TestValuesAtWalksEveryShapeAPathCanTake(t *testing.T) {
	condition := map[string]any{
		"kind": "x",
		"interface": map[string]any{
			"type":   "t",
			"engine": "postgres",
			"spec":   map[string]any{"format": "yaml"},
			"tags":   []any{"red", "blue"},
			"items":  []any{map[string]any{"part": "a"}, map[string]any{"part": "b"}, map[string]any{"other": 1}},
			"groups": []any{
				map[string]any{"members": []any{map[string]any{"role": "admin"}, map[string]any{"role": "user"}}},
				map[string]any{"members": []any{map[string]any{"role": "user"}}},
				map[string]any{"members": []any{}},
				map[string]any{},
			},
			"empty": []any{},
			"null":  nil,
		},
		"settings": map[string]any{"alternatives": []any{
			map[string]any{"env": []any{map[string]any{"property": "host"}}},
			map[string]any{"env": []any{map[string]any{"property": "port"}, map[string]any{"property": "url"}}},
		}},
	}
	cases := map[string][]string{
		"kind":                                     {"kind"},
		"interface.engine":                         {"interface.engine"},
		"interface.spec.format":                    {"interface.spec.format"},
		"interface.tags[]":                         {"interface.tags[0]", "interface.tags[1]"},
		"interface.items[].part":                   {"interface.items[0].part", "interface.items[1].part"},
		"interface.groups[].members[].role":        {"interface.groups[0].members[0].role", "interface.groups[0].members[1].role", "interface.groups[1].members[0].role"},
		"settings.alternatives[].env[].property":   {"settings.alternatives[0].env[0].property", "settings.alternatives[1].env[0].property", "settings.alternatives[1].env[1].property"},
		"interface.empty[]":                        {},
		"interface.null[]":                         {},
		"interface.missing":                        {},
		"interface.engine.deeper":                  {},
		"interface.engine[]":                       {},
		"interface.tags":                           {"interface.tags"},
		"interface.spec[]":                         {},
		"nothing.at.all":                           {},
		"interface.items[].part.deeper":            {},
		"interface.groups[].members[].role.nested": {},
	}
	for path, want := range cases {
		if got := pathsOf(valuesAt(condition, path)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}
}

func TestValuesAtNeverPanicsOnAnythingYamlCanDecodeTo(t *testing.T) {
	roots := []any{nil, "text", 3, 2.5, true, []any{}, []any{nil, "x", 1}, map[string]any{}, map[string]any{"a": nil},
		map[string]any{"a": []any{[]any{"nested list"}}}, map[string]any{"a": map[string]any{"b": []any{nil}}}}
	paths := []string{"a", "a[]", "a.b", "a[].b", "a.b[]", "a[].b[]", "", ".", "[]", "a..b", "a.[]"}
	for _, root := range roots {
		for _, path := range paths {
			valuesAt(root, path)
		}
	}
}

func TestSortedKeysIsStable(t *testing.T) {
	got := sortedKeys(map[string]any{"b": 1, "a": 2, "c": 3})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("got %v", got)
	}
	if got := sortedKeys(nil); len(got) != 0 {
		t.Fatalf("a nil map should have no keys, got %v", got)
	}
}
