package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests prove how the validator treats an extension: whatever shape a field has, it is
// checked against the extension's own definition. They run against two made-up extensions, so
// they do not depend on any real extension or on which version of it exists. The validator may
// know only what the core profile schema defines. Everything else, every shape below, comes from
// the definitions.
//
// The dummy extensions cover the ways an extension can shape a field:
//   - a string with enumerated values (interface.mode),
//   - a nested object (interface.spec.format),
//   - a list of strings (interface.tags[], interface.operations[]),
//   - a list of objects with several keys (interface.items[].part, .size, interface.operations[].action),
//   - a key of an object that is free-form (interface.operations[].target),
//   - a field of the condition itself (settings, config) with env lists and alternatives,
//   - a field with no enumerated values at all (interface.note),
//   - a dependency of one extension on another.

const dummyAlpha = `apiVersion: runtimeconditions.io/v1alpha1
kind: RuntimeConditionsExtensionDefinition
metadata:
  id: https://example.test/dummy/alpha/v1/runtimeconditions.extension.yaml
  version: v1
spec:
  kinds:
  - name: widget
  interfaceTypes:
  - {name: plain, targetKind: widget}
  - {name: listy, targetKind: widget}
  interfaceFields:
  - {name: mode, targetKind: widget, targetType: plain}
  - {name: spec, targetKind: widget, targetType: plain}
  - {name: note, targetKind: widget, targetType: plain}
  - {name: tags, targetKind: widget, targetType: listy}
  - {name: items, targetKind: widget, targetType: listy}
  conditionFields:
  - {name: settings, appliesToKinds: [widget], appliesToInterfaceTypes: [plain, listy]}
  fieldValues:
  - {field: interface.mode, targetKind: widget, targetType: plain, values: [fast, slow]}
  - {field: interface.spec.format, targetKind: widget, targetType: plain, values: [yaml, json]}
  - {field: 'interface.tags[]', targetKind: widget, targetType: listy, values: [red, blue]}
  - {field: 'interface.items[].part', targetKind: widget, targetType: listy, values: [a, b]}
  - {field: 'interface.items[].size', targetKind: widget, targetType: listy, values: [small, large]}
  - {field: 'settings.env[].property', targetKind: widget, targetType: plain, values: [host, port]}
  - {field: 'settings.alternatives[].env[].property', targetKind: widget, targetType: plain, values: [host, port]}
  - {field: 'settings.env[].property', targetKind: widget, targetType: listy, values: [host, port]}
  schemas:
  - id: widget-plain
    appliesToKind: widget
    appliesToInterfaceType: plain
    description: A plain widget names its mode, and nothing is added to its interface.
    schema:
      $schema: https://json-schema.org/draft/2020-12/schema
      type: object
      properties:
        interface:
          type: object
          required: [type, mode]
          properties:
            type: {const: plain}
            mode: {type: string}
            spec: {type: object}
            note: {type: string}
          additionalProperties: false
`

const dummyBeta = `apiVersion: runtimeconditions.io/v1alpha1
kind: RuntimeConditionsExtensionDefinition
metadata:
  id: https://example.test/dummy/beta/v2/runtimeconditions.extension.yaml
  version: v2
spec:
  dependencies: [https://example.test/dummy/alpha/v1/runtimeconditions.extension.yaml]
  kinds:
  - name: gadget
  interfaceTypes:
  - {name: strict, targetKind: gadget}
  - {name: objects, targetKind: gadget}
  interfaceFields:
  - {name: operations, targetKind: gadget, targetType: strict}
  - {name: operations, targetKind: gadget, targetType: objects}
  conditionFields:
  - {name: config, appliesToKinds: [gadget]}
  fieldValues:
  - {field: 'interface.operations[]', targetKind: gadget, targetType: strict, values: [read, write]}
  - {field: 'interface.operations[].action', targetKind: gadget, targetType: objects, values: [open, close]}
  - {field: 'config.env[].property', targetKind: gadget, targetType: strict, values: [url]}
`

const (
	dummyAlphaRef = "https://example.test/dummy/alpha:v1"
	dummyBetaRef  = "https://example.test/dummy/beta:v2"
)

func writeFiles(t *testing.T, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// dummyOptions returns the directories that hold the dummy definitions.
func dummyOptions(t *testing.T) []string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, map[string]string{
		filepath.Join(root, "alpha.yaml"): dummyAlpha,
		filepath.Join(root, "beta.yaml"):  dummyBeta,
	})
	return []string{root}
}

func dummyProfile(extensions []string, conditions ...string) []byte {
	var out strings.Builder
	out.WriteString("apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsProfile\nmetadata: {name: shapes}\nworkload: {uri: 'https://example.test/shapes'}\nextensions:\n")
	for _, extension := range extensions {
		out.WriteString("- " + extension + "\n")
	}
	out.WriteString("conditions:\n")
	for _, condition := range conditions {
		out.WriteString("- " + strings.ReplaceAll(strings.TrimRight(condition, "\n"), "\n", "\n  ") + "\n")
	}
	return []byte(out.String())
}

func TestFieldShapesAreCheckedAgainstTheExtensionDefinition(t *testing.T) {
	options := dummyOptions(t)
	alpha, both := []string{dummyAlphaRef}, []string{dummyAlphaRef, dummyBetaRef}
	cases := []struct {
		name       string
		extensions []string
		valid      string
		old, new   string // a value the extension does not allow replaces old with new
		rejected   string // text the rejection has to carry
	}{
		{"a string with enumerated values", alpha,
			"kind: widget\ninterface: {type: plain, mode: fast}\n", "fast", "turbo", "interface.mode turbo"},
		{"a nested object", alpha,
			"kind: widget\ninterface: {type: plain, mode: fast, spec: {format: yaml}}\n", "yaml", "toml", "interface.spec.format toml"},
		{"a list of strings", alpha,
			"kind: widget\ninterface: {type: listy, tags: [red, blue]}\n", "blue", "green", "interface.tags[1] green"},
		{"a list of objects with several keys", alpha,
			"kind: widget\ninterface:\n  type: listy\n  items:\n  - {part: a, size: small}\n  - {part: b, size: large}\n", "large", "huge", "interface.items[1].size huge"},
		{"a condition field with an env list", alpha,
			"kind: widget\ninterface: {type: plain, mode: fast}\nsettings:\n  env:\n  - {property: host, name: H}\n", "host", "path", "settings.env[0].property path"},
		{"a condition field with alternatives", alpha,
			"kind: widget\ninterface: {type: plain, mode: fast}\nsettings:\n  alternatives:\n  - env: [{property: host, name: H}]\n  - env: [{property: port, name: P}]\n", "port", "fax", "settings.alternatives[1].env[0].property fax"},
		{"operations as strings", both,
			"kind: gadget\ninterface: {type: strict, operations: [read, write]}\n", "write", "erase", "interface.operations[1] erase"},
		{"operations as objects keyed by something else", both,
			"kind: gadget\ninterface:\n  type: objects\n  operations:\n  - {action: open, target: anything-at-all}\n", "open", "smash", "interface.operations[0].action smash"},
		{"a condition field of another extension", both,
			"kind: gadget\ninterface: {type: strict, operations: [read]}\nconfig:\n  env:\n  - {property: url, name: U}\n", "url", "dsn", "config.env[0].property dsn"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := Profile(dummyProfile(test.extensions, test.valid), options); err != nil {
				t.Fatalf("a profile the extensions allow was rejected: %v", err)
			}
			broken := strings.Replace(test.valid, test.old, test.new, 1)
			err := Profile(dummyProfile(test.extensions, broken), options)
			if err == nil || !strings.Contains(err.Error(), test.rejected) {
				t.Fatalf("a value the extension does not allow should be rejected with %q, got %v\n%s", test.rejected, err, broken)
			}
		})
	}
}

func TestAFieldWithNoEnumeratedValuesAcceptsAnyValue(t *testing.T) {
	// interface.note and interface.operations[].target are declared, but no extension lists their values.
	options := dummyOptions(t)
	for _, profile := range [][]byte{
		dummyProfile([]string{dummyAlphaRef}, "kind: widget\ninterface: {type: plain, mode: fast, note: whatever}\n"),
		dummyProfile([]string{dummyAlphaRef, dummyBetaRef}, "kind: gadget\ninterface: {type: objects, operations: [{action: close, target: x}, {action: close, target: y}]}\n"),
	} {
		if err := Profile(profile, options); err != nil {
			t.Fatalf("a free-form value was rejected: %v", err)
		}
	}
}

func TestAFieldHasToBeDeclaredForTheKindAndInterfaceTypeItIsUsedOn(t *testing.T) {
	options := dummyOptions(t)
	cases := map[string]string{
		// tags is declared for the listy interface, not the plain one.
		"an interface field of another interface type": "kind: widget\ninterface: {type: plain, mode: fast, tags: [red]}\n",
		// settings is declared for widgets, not gadgets.
		"a condition field of another kind": "kind: gadget\ninterface: {type: strict, operations: [read]}\nsettings:\n  env:\n  - {property: host, name: H}\n",
		"an interface type of another kind": "kind: widget\ninterface: {type: strict, operations: [read]}\n",
		"a kind no extension defines":       "kind: gizmo\ninterface: {type: plain}\n",
	}
	for name, condition := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Profile(dummyProfile([]string{dummyAlphaRef, dummyBetaRef}, condition), options); err == nil {
				t.Fatalf("a field that is not declared for its use was accepted:\n%s", condition)
			}
		})
	}
}

func TestAFieldNoExtensionDeclaresIsLeftToTheExtensionSchemas(t *testing.T) {
	options := dummyOptions(t)
	// Nothing declares colour, so the vocabulary has no opinion. The plain widget schema forbids it ...
	plain := dummyProfile([]string{dummyAlphaRef}, "kind: widget\ninterface: {type: plain, mode: fast, colour: red}\n")
	if err := Profile(plain, options); err == nil || !strings.Contains(err.Error(), "widget-plain") {
		t.Fatalf("the extension schema should have rejected the unknown field, got %v", err)
	}
	// ... and the listy widget has no schema, so it is accepted.
	listy := dummyProfile([]string{dummyAlphaRef}, "kind: widget\ninterface: {type: listy, tags: [red], colour: red}\n")
	if err := Profile(listy, options); err != nil {
		t.Fatalf("a field no extension defines should not fail the vocabulary check: %v", err)
	}
}

func TestAValueOfTheWrongTypeIsLeftToTheExtensionSchemas(t *testing.T) {
	// The vocabulary compares strings. A mapping where a string belongs is the schema's to reject.
	options := dummyOptions(t)
	profile := dummyProfile([]string{dummyAlphaRef}, "kind: widget\ninterface: {type: plain, mode: {nested: true}}\n")
	err := Profile(profile, options)
	if err == nil || !strings.Contains(err.Error(), "widget-plain") {
		t.Fatalf("expected the extension schema to reject the value, got %v", err)
	}
}

func TestAProfileHasToNameTheWholeDependencyClosure(t *testing.T) {
	options := dummyOptions(t)
	err := Profile(dummyProfile([]string{dummyBetaRef}, "kind: gadget\ninterface: {type: strict, operations: [read]}\n"), options)
	if err == nil || !strings.Contains(err.Error(), "missing dependency") {
		t.Fatalf("a profile that leaves out a dependency should be rejected, got %v", err)
	}
}

func TestSeveralExtensionsWithDifferentShapesCanBeUsedInOneProfile(t *testing.T) {
	options := dummyOptions(t)
	profile := dummyProfile([]string{dummyAlphaRef, dummyBetaRef},
		"kind: widget\ninterface: {type: plain, mode: slow, spec: {format: json}}\nsettings:\n  env:\n  - {property: port, name: P}\n",
		"kind: widget\ninterface:\n  type: listy\n  items:\n  - {part: a, size: small}\n",
		"kind: gadget\ninterface: {type: strict, operations: [read]}\nconfig:\n  env:\n  - {property: url, name: U}\n",
		"kind: gadget\ninterface: {type: objects, operations: [{action: open, target: t}]}\n",
	)
	if err := Profile(profile, options); err != nil {
		t.Fatalf("a profile that mixes extensions was rejected: %v", err)
	}
}
