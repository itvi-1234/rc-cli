package validate

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// More ways an extension can shape its fields, and the combinations of them. A new extension, or
// a new shape in an existing one, must never make the validator fail on a profile the extension
// allows, or accept one it does not. gamma adds the shapes alpha and beta do not have; delta
// exists to define the same value twice.

const dummyGamma = `apiVersion: runtimeconditions.io/v1alpha1
kind: RuntimeConditionsExtensionDefinition
metadata:
  id: https://example.test/dummy/gamma/v3/runtimeconditions.extension.yaml
  version: v3
spec:
  kinds:
  - name: thing
  interfaceTypes:
  - {name: web, targetKind: thing}
  - {name: db, targetKind: thing}
  - {name: queue, targetKind: thing}
  interfaceFields:
  - {name: groups, targetKind: thing, targetType: web}
  - {name: mode, targetKind: thing, targetType: web}
  - {name: mode, targetKind: thing, targetType: db}
  - {name: matrix, targetKind: thing, targetType: queue}
  conditionFields:
  - {name: limits, appliesToKinds: [thing]}
  fieldValues:
  - {field: 'interface.groups[].members[].role', targetKind: thing, targetType: web, values: [admin, user]}
  - {field: interface.mode, targetKind: thing, targetType: web, values: [live, test]}
  - {field: interface.mode, targetKind: thing, targetType: db, values: [read, write]}
  - {field: 'limits.env[].property', targetKind: thing, targetType: web, values: [cpu]}
  - {field: 'limits.env[].property', targetKind: thing, targetType: db, values: [memory]}
  - {field: 'limits.env[].property', targetKind: thing, targetType: queue, values: [depth]}
`

// delta allows the value fast for alpha's interface.mode a second time.
const dummyDelta = `apiVersion: runtimeconditions.io/v1alpha1
kind: RuntimeConditionsExtensionDefinition
metadata:
  id: https://example.test/dummy/delta/v1/runtimeconditions.extension.yaml
  version: v1
spec:
  dependencies: [https://example.test/dummy/alpha/v1/runtimeconditions.extension.yaml]
  fieldValues:
  - {field: interface.mode, targetKind: widget, targetType: plain, values: [fast, turbo]}
`

const (
	dummyGammaRef = "https://example.test/dummy/gamma:v3"
	dummyDeltaRef = "https://example.test/dummy/delta:v1"
)

func writeDefinitions(tb testing.TB, root string, definitions ...string) {
	tb.Helper()
	for index, definition := range definitions {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("definition-%d.yaml", index)), []byte(definition), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}

func optionsWith(t *testing.T, extra ...string) []string {
	t.Helper()
	root := t.TempDir()
	writeDefinitions(t, root, append([]string{dummyAlpha, dummyBeta, dummyGamma}, extra...)...)
	return []string{root}
}

var allDummies = []string{dummyAlphaRef, dummyBetaRef, dummyGammaRef}

func TestListsNestedInsideListsAreWalked(t *testing.T) {
	options := optionsWith(t)
	valid := "kind: thing\ninterface:\n  type: web\n  groups:\n  - members: [{role: admin}, {role: user}]\n  - members: [{role: user}]\n  - members: []\n  - {}\n"
	if err := Profile(dummyProfile(allDummies, valid), options); err != nil {
		t.Fatalf("nested lists were rejected: %v", err)
	}
	broken := strings.Replace(valid, "{role: user}]\n  - members: [{role: user}]", "{role: root}]\n  - members: [{role: user}]", 1)
	err := Profile(dummyProfile(allDummies, broken), options)
	if err == nil || !strings.Contains(err.Error(), "interface.groups[0].members[1].role root") {
		t.Fatalf("a bad value deep in nested lists should be reported where it is, got %v", err)
	}
}

func TestTheSameFieldNameCanAllowDifferentValuesOnDifferentInterfaceTypes(t *testing.T) {
	options := optionsWith(t)
	accepted := map[string]string{
		"db":     "kind: thing\ninterface: {type: db, mode: read}\n",
		"web":    "kind: thing\ninterface: {type: web, mode: live}\n",
		"widget": "kind: widget\ninterface: {type: plain, mode: fast}\n",
	}
	rejected := map[string]string{
		"db takes db values only":     "kind: thing\ninterface: {type: db, mode: live}\n",
		"web takes web values only":   "kind: thing\ninterface: {type: web, mode: read}\n",
		"widget takes widget values":  "kind: widget\ninterface: {type: plain, mode: read}\n",
		"a value of another kind too": "kind: thing\ninterface: {type: db, mode: fast}\n",
	}
	for name, condition := range accepted {
		if err := Profile(dummyProfile(allDummies, condition), options); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, condition := range rejected {
		if err := Profile(dummyProfile(allDummies, condition), options); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAConditionFieldCanBeDefinedWithTheSameNameForEveryInterfaceType(t *testing.T) {
	options := optionsWith(t)
	for interfaceType, property := range map[string]string{"web": "cpu", "db": "memory", "queue": "depth"} {
		condition := "kind: thing\ninterface: {type: " + interfaceType + "}\nlimits:\n  env:\n  - {property: " + property + ", name: X}\n"
		if err := Profile(dummyProfile(allDummies, condition), options); err != nil {
			t.Errorf("%s: %v", interfaceType, err)
		}
		wrong := strings.Replace(condition, property, "disk", 1)
		if err := Profile(dummyProfile(allDummies, wrong), options); err == nil {
			t.Errorf("%s: a value outside the type's list was accepted", interfaceType)
		}
	}
}

func TestTwoExtensionsDefiningTheSameValueForTheSameFieldIsAConflict(t *testing.T) {
	// delta repeats the value fast that alpha already allows for interface.mode. The conflict is in
	// the set of extensions, so every profile that uses both is rejected, whatever value it holds.
	options := optionsWith(t, dummyDelta)
	extensions := append([]string{dummyDeltaRef}, allDummies...)
	for _, mode := range []string{"fast", "turbo", "slow"} {
		err := Profile(dummyProfile(extensions, "kind: widget\ninterface: {type: plain, mode: "+mode+"}\n"), options)
		if err == nil || !strings.Contains(err.Error(), "vocabulary conflict") {
			t.Fatalf("mode %s: a value defined twice should be reported as a conflict, got %v", mode, err)
		}
	}
	// without delta the same profile is fine
	if err := Profile(dummyProfile(allDummies, "kind: widget\ninterface: {type: plain, mode: fast}\n"), optionsWith(t)); err != nil {
		t.Fatalf("alpha on its own was rejected: %v", err)
	}
}

func TestAKindFromAnExtensionTheProfileDoesNotDeclareIsRejected(t *testing.T) {
	options := optionsWith(t)
	err := Profile(dummyProfile([]string{dummyAlphaRef}, "kind: thing\ninterface: {type: web}\n"), options)
	if err == nil || !strings.Contains(err.Error(), "kind thing") {
		t.Fatalf("expected the kind to be unknown, got %v", err)
	}
}

func TestDeclaringAnExtensionNoConditionUsesIsFine(t *testing.T) {
	options := optionsWith(t)
	if err := Profile(dummyProfile(allDummies, "kind: widget\ninterface: {type: plain, mode: fast}\n"), options); err != nil {
		t.Fatal(err)
	}
}

func TestAProfileWithNoConditionsAndNoExtensionsIsValid(t *testing.T) {
	options := optionsWith(t)
	profile := []byte("apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsProfile\nmetadata: {name: empty}\nworkload: {uri: 'https://example.test/e'}\nextensions: []\nconditions: []\n")
	if err := Profile(profile, options); err != nil {
		t.Fatal(err)
	}
}

func TestExtensionReferencesMayBeWrittenInEveryAcceptedForm(t *testing.T) {
	options := optionsWith(t)
	for _, ref := range []string{
		"https://example.test/dummy/alpha:v1",
		"https://example.test/dummy/alpha/v1/runtimeconditions.extension.yaml",
		"https://example.test/dummy/alpha/v1/runtimeconditions.extension.yaml:v1",
	} {
		profile := dummyProfile([]string{ref}, "kind: widget\ninterface: {type: plain, mode: fast}\n")
		if err := Profile(profile, options); err != nil {
			t.Errorf("%s: %v", ref, err)
		}
	}
}

func TestMalformedConditionsAreReportedAndNeverPanic(t *testing.T) {
	options := optionsWith(t)
	for name, condition := range map[string]string{
		"a string":                "just text\n",
		"a list":                  "[a, b]\n",
		"null":                    "null\n",
		"a number":                "7\n",
		"no kind":                 "interface: {type: plain}\n",
		"a number for the kind":   "kind: 5\ninterface: {type: plain}\n",
		"a list for the kind":     "kind: [widget]\ninterface: {type: plain}\n",
		"no interface":            "kind: widget\n",
		"a string interface":      "kind: widget\ninterface: plain\n",
		"a list interface":        "kind: widget\ninterface: [plain]\n",
		"no interface type":       "kind: widget\ninterface: {mode: fast}\n",
		"a number interface type": "kind: widget\ninterface: {type: 3}\n",
		"a null interface":        "kind: widget\ninterface:\n",
		"an empty mapping":        "{}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := Profile(dummyProfile([]string{dummyAlphaRef}, condition), options); err == nil {
				t.Fatalf("a malformed condition was accepted:\n%s", condition)
			}
		})
	}
}

func TestValuesOfOtherTypesAtAnEnumeratedPathNeverPanic(t *testing.T) {
	// listy has no schema, so only the vocabulary looks at it. A value that is not a string is not
	// compared with the list. It is for the extension's schema to reject, and listy has none.
	options := optionsWith(t)
	for name, tags := range map[string]string{
		"numbers": "[1, 2]", "booleans": "[true, false]", "nulls": "[null]", "objects": "[{a: 1}]",
		"lists": "[[red]]", "mixed with a bad string": "[1, green]",
	} {
		t.Run(name, func(t *testing.T) {
			err := Profile(dummyProfile([]string{dummyAlphaRef}, "kind: widget\ninterface: {type: listy, tags: "+tags+"}\n"), options)
			if strings.Contains(name, "bad string") != (err != nil) {
				t.Fatalf("unexpected result for %s: %v", tags, err)
			}
		})
	}
}

func TestUnicodeAndEmptyValuesAreCheckedLikeAnyOther(t *testing.T) {
	options := optionsWith(t)
	for _, value := range []string{"'ß'", "'日本語'", "''", "'red '", "Red"} {
		condition := "kind: widget\ninterface: {type: listy, tags: [" + value + "]}\n"
		if err := Profile(dummyProfile([]string{dummyAlphaRef}, condition), options); err == nil {
			t.Errorf("%s was accepted but is not in the list", value)
		}
	}
}

func TestManyConditionsAreEachReportedWithTheirOwnIndex(t *testing.T) {
	options := optionsWith(t)
	conditions := make([]string, 0, 50)
	for index := 0; index < 50; index++ {
		mode := "fast"
		if index == 37 {
			mode = "broken"
		}
		conditions = append(conditions, "kind: widget\ninterface: {type: plain, mode: "+mode+"}\n")
	}
	err := Profile(dummyProfile([]string{dummyAlphaRef}, conditions...), options)
	if err == nil || !strings.Contains(err.Error(), "conditions[37].interface.mode broken") || strings.Contains(err.Error(), "conditions[36]") {
		t.Fatalf("the bad condition should be the only one reported, got %v", err)
	}
}

// shapes lists one value at each place the dummy extensions enumerate values: how to write the
// field in a condition, the values that are allowed, and one that is not.
type shape struct {
	kind, interfaceType string
	write               func(value string) string
	good                []string
	bad                 string
	extensions          []string
}

var shapes = []shape{
	{"widget", "plain", func(v string) string { return "mode: " + v }, []string{"fast", "slow"}, "turbo", []string{dummyAlphaRef}},
	{"widget", "plain", func(v string) string { return "mode: fast\n  spec: {format: " + v + "}" }, []string{"yaml", "json"}, "toml", []string{dummyAlphaRef}},
	{"widget", "listy", func(v string) string { return "tags: [red, " + v + "]" }, []string{"red", "blue"}, "green", []string{dummyAlphaRef}},
	{"widget", "listy", func(v string) string { return "items: [{part: a, size: " + v + "}]" }, []string{"small", "large"}, "huge", []string{dummyAlphaRef}},
	{"gadget", "strict", func(v string) string { return "operations: [read, " + v + "]" }, []string{"read", "write"}, "erase", []string{dummyAlphaRef, dummyBetaRef}},
	{"gadget", "objects", func(v string) string { return "operations: [{action: " + v + ", target: anything}]" }, []string{"open", "close"}, "smash", []string{dummyAlphaRef, dummyBetaRef}},
	{"thing", "web", func(v string) string { return "groups: [{members: [{role: " + v + "}]}]" }, []string{"admin", "user"}, "root", allDummies},
	{"thing", "web", func(v string) string { return "mode: " + v }, []string{"live", "test"}, "read", allDummies},
	{"thing", "db", func(v string) string { return "mode: " + v }, []string{"read", "write"}, "live", allDummies},
}

// TestRandomCombinationsOfShapesAreValidExactlyWhenEveryValueIsAllowed builds profiles at random
// from the shapes above, with conditions of different extensions side by side, and checks that
// the validator accepts a profile exactly when every value in it is one the extension allows.
func TestRandomCombinationsOfShapesAreValidExactlyWhenEveryValueIsAllowed(t *testing.T) {
	options := optionsWith(t)
	random := rand.New(rand.NewSource(20261114))
	for round := 0; round < 400; round++ {
		count := 1 + random.Intn(6)
		conditions := make([]string, 0, count)
		wantValid := true
		for ; count > 0; count-- {
			picked := shapes[random.Intn(len(shapes))]
			value := picked.good[random.Intn(len(picked.good))]
			if random.Intn(4) == 0 {
				value, wantValid = picked.bad, false
			}
			conditions = append(conditions, "kind: "+picked.kind+"\ninterface:\n  type: "+picked.interfaceType+"\n  "+picked.write(value)+"\n")
		}
		err := Profile(dummyProfile(allDummies, conditions...), options)
		if wantValid != (err == nil) {
			t.Fatalf("round %d: valid=%v but got %v\n%s", round, wantValid, err, strings.Join(conditions, "---\n"))
		}
	}
}

// FuzzValidateProfileYAML only checks that no input makes the validator panic. A plain go test runs
// the seeds; go test -fuzz=FuzzValidateProfileYAML explores further.
func FuzzProfile(f *testing.F) {
	f.Add("kind: widget\ninterface: {type: plain, mode: fast}\n")
	f.Add("kind: gadget\ninterface: {type: strict, operations: [read]}\nconfig: {env: [{property: url, name: U}]}\n")
	f.Add("kind: thing\ninterface:\n  type: web\n  groups: [{members: [{role: admin}]}]\n")
	f.Add("- - - 1\n")
	f.Add("{kind: {a: b}, interface: [1, [2]]}\n")
	root := f.TempDir()
	writeDefinitions(f, root, dummyAlpha, dummyBeta, dummyGamma)
	options := []string{root}
	f.Fuzz(func(t *testing.T, condition string) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("the validator panicked on %q: %v", condition, recovered)
			}
		}()
		profile := "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsProfile\nextensions:\n- " + dummyAlphaRef + "\n- " + dummyBetaRef + "\n- " + dummyGammaRef + "\nconditions:\n- " + strings.ReplaceAll(condition, "\n", "\n  ") + "\n"
		_ = Profile([]byte(profile), options)
	})
}
