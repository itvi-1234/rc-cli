package validate

import (
	"path/filepath"
	"strings"
	"testing"
)

// minimalProfile declares the given extensions and has no conditions.
func minimalProfile(extensions ...string) []byte {
	var out strings.Builder
	out.WriteString("apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsProfile\nmetadata: {name: p}\nworkload: {uri: 'https://example.test/p'}\nextensions:\n")
	for _, extension := range extensions {
		out.WriteString("- " + extension + "\n")
	}
	out.WriteString("conditions: []\n")
	return []byte(out.String())
}

func definition(id, version, spec string) string {
	return "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsExtensionDefinition\nmetadata: {id: " + id + ", version: " + version + "}\nspec:\n" + spec
}

func catalogWith(t *testing.T, files map[string]string) []string {
	t.Helper()
	root := t.TempDir()
	written := map[string]string{}
	for name, content := range files {
		written[filepath.Join(root, name)] = content
	}
	writeFiles(t, written)
	return []string{root}
}

const (
	extA = "https://example.test/x/a/v1/runtimeconditions.extension.yaml"
	extB = "https://example.test/x/b/v1/runtimeconditions.extension.yaml"
	extC = "https://example.test/x/c/v1/runtimeconditions.extension.yaml"
)

func TestADefinitionIsCheckedAgainstItselfWhenAProfileDeclaresIt(t *testing.T) {
	kinds := "  kinds: [{name: w}]\n"
	cases := map[string]struct{ file, want string }{
		"a wrong apiVersion":                       {strings.Replace(definition(extA, "v1", kinds), "v1alpha1", "v9", 1), "apiVersion must be runtimeconditions.io/v1alpha1"},
		"nothing defined":                          {definition(extA, "v1", "  dependencies: []\n"), "spec must define at least one vocabulary item or schema"},
		"a kind twice":                             {definition(extA, "v1", "  kinds: [{name: w}, {name: w}]\n"), "duplicate vocabulary definition kind:w"},
		"a kind without a name":                    {definition(extA, "v1", "  kinds: [{name: ''}]\n"), "kind name is required"},
		"an interface type without a kind":         {definition(extA, "v1", kinds+"  interfaceTypes: [{name: t}]\n"), "interfaceTypes entries require name and targetKind"},
		"an interface type of an unknown kind":     {definition(extA, "v1", kinds+"  interfaceTypes: [{name: t, targetKind: nope}]\n"), "interface type t targetKind nope: expected exactly one definition, got 0"},
		"an interface field of an unknown type":    {definition(extA, "v1", kinds+"  interfaceTypes: [{name: t, targetKind: w}]\n  interfaceFields: [{name: f, targetKind: w, targetType: nope}]\n"), "interface field f targetType w/nope"},
		"a condition field with no kinds":          {definition(extA, "v1", kinds+"  conditionFields: [{name: c}]\n"), "condition field c must declare appliesToKinds"},
		"field values without values":              {definition(extA, "v1", kinds+"  interfaceTypes: [{name: t, targetKind: w}]\n  fieldValues: [{field: interface.x, targetKind: w, targetType: t, values: []}]\n"), "fieldValues entries require field, targetKind, and values"},
		"field values for a field nobody declares": {definition(extA, "v1", kinds+"  interfaceTypes: [{name: t, targetKind: w}]\n  fieldValues: [{field: interface.x, targetKind: w, targetType: t, values: [a]}]\n"), "fieldValues field interface.x interface field w/t/x"},
		"a repeated value":                         {definition(extA, "v1", kinds+"  interfaceTypes: [{name: t, targetKind: w}]\n  interfaceFields: [{name: x, targetKind: w, targetType: t}]\n  fieldValues: [{field: interface.x, targetKind: w, targetType: t, values: [a, a]}]\n"), `duplicate field value "a" for interface.x`},
		"a schema without a description":           {definition(extA, "v1", kinds+"  schemas: [{id: s, schema: {type: object}}]\n"), "schema s description is required"},
		"a schema that does not compile":           {definition(extA, "v1", kinds+"  schemas: [{id: s, description: d, schema: {type: 12}}]\n"), "schema s is invalid"},
		"a schema for an unknown kind":             {definition(extA, "v1", kinds+"  schemas: [{id: s, description: d, appliesToKind: nope, schema: {type: object}}]\n"), "schema s appliesToKind nope"},
		"a dependency that is not in the catalog":  {definition(extA, "v1", kinds+"  dependencies: ["+extB+"]\n"), "missing extension definition for dependency"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			roots := catalogWith(t, map[string]string{"a.yaml": test.file})
			err := Profile(minimalProfile("https://example.test/x/a:v1"), roots)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestADefinitionThatDependsOnItselfThroughOthersIsReportedNotFollowed(t *testing.T) {
	kinds := "  kinds: [{name: %s}]\n"
	roots := catalogWith(t, map[string]string{
		"a.yaml": definition(extA, "v1", strings.Replace(kinds, "%s", "a", 1)+"  dependencies: ["+extB+"]\n"),
		"b.yaml": definition(extB, "v1", strings.Replace(kinds, "%s", "b", 1)+"  dependencies: ["+extC+"]\n"),
		"c.yaml": definition(extC, "v1", strings.Replace(kinds, "%s", "c", 1)+"  dependencies: ["+extA+"]\n"),
	})
	err := Profile(minimalProfile("https://example.test/x/a:v1", "https://example.test/x/b:v1", "https://example.test/x/c:v1"), roots)
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected a dependency cycle, got %v", err)
	}
}

func TestEachDefinitionIsReadFromWhereverItLives(t *testing.T) {
	kinds := "  kinds: [{name: w}]\n"
	roots := catalogWith(t, map[string]string{
		"deep/er/a.yaml":      definition(extA, "v1", kinds),
		"other.yml":           definition(extB, "v1", "  kinds: [{name: b}]\n"),
		"notes.yaml":          "just: notes\n",
		"node_modules/x.yaml": definition(extC, "v1", "  kinds: [{name: c}]\n"),
		"readme.txt":          "not yaml at all",
		".git/ignored.yaml":   definition(extC, "v1", "  kinds: [{name: c}]\n"),
	})
	for _, ref := range []string{"https://example.test/x/a:v1", "https://example.test/x/b:v1"} {
		if err := Profile(minimalProfile(ref), roots); err != nil {
			t.Errorf("%s was not found: %v", ref, err)
		}
	}
	for _, ref := range []string{"https://example.test/x/c:v1"} {
		if err := Profile(minimalProfile(ref), roots); err == nil {
			t.Errorf("%s lives in a directory that is skipped but was found", ref)
		}
	}
}

func TestSeveralDirectoriesAndRepeatedOnesAreRead(t *testing.T) {
	first := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n")})
	second := catalogWith(t, map[string]string{"b.yaml": definition(extB, "v1", "  kinds: [{name: b}]\n")})
	roots := append(append(first, second...), first...)
	if err := Profile(minimalProfile("https://example.test/x/a:v1", "https://example.test/x/b:v1"), roots); err != nil {
		t.Fatal(err)
	}
}

func TestTheSameReleaseDefinedTwiceIsReported(t *testing.T) {
	roots := catalogWith(t, map[string]string{
		"one.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n"),
		"two.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n"),
	})
	err := Profile(minimalProfile("https://example.test/x/a:v1"), roots)
	if err == nil || !strings.Contains(err.Error(), "duplicate extension release") {
		t.Fatalf("expected a duplicate release, got %v", err)
	}
}

func TestAFileThatIsNotValidYAMLIsReportedAndTheRestStillRead(t *testing.T) {
	roots := catalogWith(t, map[string]string{
		"broken.yaml": "kind: [unclosed\n",
		"a.yaml":      definition(extA, "v1", "  kinds: [{name: a}]\n"),
	})
	err := Profile(minimalProfile("https://example.test/x/a:v1"), roots)
	if err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Fatalf("expected the broken file to be named, got %v", err)
	}
	if strings.Contains(err.Error(), "missing extension definition") {
		t.Fatalf("the good definition should still have been read: %v", err)
	}
}

func TestADefinitionWithoutAUsableIDIsReported(t *testing.T) {
	for name, file := range map[string]string{
		"no id":                 "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsExtensionDefinition\nmetadata: {version: v1}\nspec:\n  kinds: [{name: a}]\n",
		"a numeric version":     "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsExtensionDefinition\nmetadata: {id: " + extA + ", version: 3}\nspec:\n  kinds: [{name: a}]\n",
		"an obsolete uri field": "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsExtensionDefinition\nmetadata: {id: " + extA + ", uri: x}\nspec:\n  kinds: [{name: a}]\n",
	} {
		t.Run(name, func(t *testing.T) {
			roots := catalogWith(t, map[string]string{"a.yaml": file})
			if err := Profile(minimalProfile("https://example.test/x/a:v1"), roots); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestADefinitionKeyTheReaderDoesNotKnowIsIgnored(t *testing.T) {
	roots := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a, note: ignored}]\n  somethingNew: {x: 1}\n")})
	if err := Profile(minimalProfile("https://example.test/x/a:v1"), roots); err != nil {
		t.Fatalf("an unknown key should be ignored: %v", err)
	}
}

func TestOneMessageIsShownAloneAndSeveralUnderAHeading(t *testing.T) {
	roots := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n")})
	one := Profile(minimalProfile("https://example.test/x/zz:v1"), roots)
	if one == nil || strings.Contains(one.Error(), "profile validation failed") || !strings.Contains(one.Error(), "missing extension definition") {
		t.Fatalf("a single problem should be shown on its own, got %v", one)
	}
	two := strings.Replace(string(minimalProfile("https://example.test/x/zz:v1")), "runtimeconditions.io/v1alpha1", "nope", 1)
	many := Profile([]byte(two), roots)
	if many == nil || !strings.HasPrefix(many.Error(), "profile validation failed:\n- ") {
		t.Fatalf("several problems should be listed under a heading, got %v", many)
	}
}

const coreHead = "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsProfile\n"

func TestTheCoreFieldsOfAProfileAreCheckedAgainstTheCoreSchema(t *testing.T) {
	roots := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n  interfaceTypes: [{name: t, targetKind: a}]\n")})
	ext := "extensions: [https://example.test/x/a:v1]\n"
	body := "metadata: {name: p}\nworkload: {uri: 'https://example.test/p'}\n"
	condition := "conditions:\n- {kind: a, interface: {type: t}}\n"
	valid := coreHead + body + ext + condition
	if err := Profile([]byte(valid), roots); err != nil {
		t.Fatalf("a profile with the right core fields was rejected: %v", err)
	}
	cases := map[string]struct{ profile, want string }{
		"a wrong apiVersion":                  {strings.Replace(valid, "v1alpha1", "v9", 1), "/apiVersion"},
		"a wrong kind":                        {strings.Replace(valid, "RuntimeConditionsProfile", "Other", 1), "/kind"},
		"no metadata":                         {coreHead + "workload: {uri: x}\n" + ext + condition, "metadata"},
		"a metadata without a name":           {coreHead + "metadata: {}\nworkload: {uri: x}\n" + ext + condition, "name"},
		"an empty metadata name":              {coreHead + "metadata: {name: ''}\nworkload: {uri: x}\n" + ext + condition, "/metadata/name"},
		"an extra key in metadata":            {coreHead + "metadata: {name: p, owner: me}\nworkload: {uri: x}\n" + ext + condition, "owner"},
		"no workload":                         {coreHead + "metadata: {name: p}\n" + ext + condition, "workload"},
		"a workload without a uri":            {coreHead + "metadata: {name: p}\nworkload: {version: v1}\n" + ext + condition, "uri"},
		"an extra key in workload":            {coreHead + "metadata: {name: p}\nworkload: {uri: x, owner: me}\n" + ext + condition, "owner"},
		"an extra top-level key":              {valid + "surprise: 1\n", "surprise"},
		"no extensions list":                  {coreHead + body + condition, "extensions"},
		"conditions with an empty extensions": {coreHead + body + "extensions: []\n" + condition, "extensions"},
		"an extension written as an object":   {coreHead + body + "extensions: [{id: https://example.test/x/a/v1/runtimeconditions.extension.yaml, version: v1}]\n" + condition, "/extensions/0"},
		"the same extension twice":            {coreHead + body + "extensions: [https://example.test/x/a:v1, https://example.test/x/a:v1]\n" + condition, "extensions"},
		"a condition without a kind":          {coreHead + body + ext + "conditions:\n- {interface: {type: t}}\n", "kind"},
		"a condition without an interface":    {coreHead + body + ext + "conditions:\n- {kind: a}\n", "interface"},
		"an interface without a type":         {coreHead + body + ext + "conditions:\n- {kind: a, interface: {}}\n", "type"},
		"no conditions list":                  {coreHead + body + ext, "conditions"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			err := Profile([]byte(test.profile), roots)
			if err == nil || !strings.Contains(err.Error(), "core profile schema") || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected a core schema error mentioning %q, got %v", test.want, err)
			}
		})
	}
}

func TestTheCoreSchemaAndTheExtensionsAreBothReported(t *testing.T) {
	roots := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n  interfaceTypes: [{name: t, targetKind: a}]\n")})
	profile := coreHead + "workload: {uri: x}\nextensions: [https://example.test/x/a:v1]\nconditions:\n- {kind: zz, interface: {type: t}}\n"
	err := Profile([]byte(profile), roots)
	if err == nil || !strings.Contains(err.Error(), "core profile schema") || !strings.Contains(err.Error(), "kind zz") {
		t.Fatalf("both the missing metadata and the unknown kind should be reported, got %v", err)
	}
}

func TestWithoutACoreSchemaTheValidatorFailsClosed(t *testing.T) {
	roots := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n")})
	saved := coreSchemaOverride
	coreSchemaOverride = nil
	defer func() { coreSchemaOverride = saved }()
	err := Profile(minimalProfile("https://example.test/x/a:v1"), roots)
	if err == nil || !strings.Contains(err.Error(), "no verified core profile schema") {
		t.Fatalf("a build without the core schema must not call a profile valid, got %v", err)
	}
}

func TestABareIDNamesTheOneDefinitionWithThatIDAndTwoAreAmbiguous(t *testing.T) {
	one := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n")})
	if err := Profile(minimalProfile(extA), one); err != nil {
		t.Fatalf("a bare id should find its only definition: %v", err)
	}
	two := catalogWith(t, map[string]string{
		"a1.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n"),
		"a2.yaml": definition(extA, "v2", "  kinds: [{name: a2}]\n"),
	})
	err := Profile(minimalProfile(extA), two)
	if err == nil || !strings.Contains(err.Error(), "more than one extension definition") {
		t.Fatalf("a bare id with two versions should be ambiguous, got %v", err)
	}
}

func TestAProfileThatIsNotYAMLOrNotAMappingIsRejected(t *testing.T) {
	roots := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n")})
	for name, profile := range map[string]string{"unbalanced": "extensions: [a\n", "a list": "- a\n- b\n", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			if err := Profile([]byte(profile), roots); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// delta-style extension that adds a condition field to alpha's widgets.
const dummyExtras = `apiVersion: runtimeconditions.io/v1alpha1
kind: RuntimeConditionsExtensionDefinition
metadata: {id: https://example.test/dummy/extras/v1/runtimeconditions.extension.yaml, version: v1}
spec:
  dependencies: [https://example.test/dummy/alpha/v1/runtimeconditions.extension.yaml]
  conditionFields:
  - {name: extras, appliesToKinds: [widget]}
`

func TestAConditionFieldUsedWithoutDeclaringTheExtensionThatDefinesItIsRejected(t *testing.T) {
	options := optionsWith(t, dummyExtras)
	condition := "kind: widget\ninterface: {type: plain, mode: fast}\nextras: {anything: 1}\n"
	if err := Profile(dummyProfile([]string{dummyAlphaRef, "https://example.test/dummy/extras:v1"}, condition), options); err != nil {
		t.Fatalf("the field was rejected although its extension is declared: %v", err)
	}
	err := Profile(dummyProfile([]string{dummyAlphaRef}, condition), options)
	if err == nil || !strings.Contains(err.Error(), "conditions[0].extras for widget/plain: expected exactly one definition, got 0") {
		t.Fatalf("a condition field no declared extension defines should be rejected, got %v", err)
	}
}

func TestACoreConditionFieldNeedsNoExtension(t *testing.T) {
	options := optionsWith(t)
	condition := "name: mine\noptional: true\nextension: " + dummyAlphaRef + "\nkind: widget\ninterface: {type: plain, mode: fast}\n"
	if err := Profile(dummyProfile([]string{dummyAlphaRef}, condition), options); err != nil {
		t.Fatalf("name, optional and extension belong to the core schema: %v", err)
	}
}

func TestAnInterfaceFieldOfAnotherInterfaceTypeIsRejectedByTheVocabularyAlone(t *testing.T) {
	// mode is declared for the plain widget, not the listy one. The listy widget has no schema, so
	// nothing but the vocabulary can notice.
	options := optionsWith(t)
	err := Profile(dummyProfile([]string{dummyAlphaRef}, "kind: widget\ninterface: {type: listy, mode: fast}\n"), options)
	if err == nil || !strings.Contains(err.Error(), "interface.mode for widget/listy: expected exactly one definition, got 0") {
		t.Fatalf("expected the field to be rejected for the listy widget, got %v", err)
	}
}

func TestAnEmptyConditionsListIsValid(t *testing.T) {
	roots := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n")})
	body := coreHead + "metadata: {name: p}\nworkload: {uri: 'https://example.test/p'}\n"
	for name, profile := range map[string]string{
		"with no extensions":         body + "extensions: []\nconditions: []\n",
		"with an extension declared": body + "extensions: [https://example.test/x/a:v1]\nconditions: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := Profile([]byte(profile), roots); err != nil {
				t.Fatalf("a profile with an empty conditions list was rejected: %v", err)
			}
		})
	}
}

func TestConditionsHaveToDeclareExtensionsButOnlyWhenThereAreSome(t *testing.T) {
	roots := catalogWith(t, map[string]string{"a.yaml": definition(extA, "v1", "  kinds: [{name: a}]\n  interfaceTypes: [{name: t, targetKind: a}]\n")})
	body := coreHead + "metadata: {name: p}\nworkload: {uri: 'https://example.test/p'}\n"
	withCondition := body + "extensions: []\nconditions:\n- {kind: a, interface: {type: t}}\n"
	if err := Profile([]byte(withCondition), roots); err == nil {
		t.Fatal("a condition with no declared extensions was accepted")
	}
}
