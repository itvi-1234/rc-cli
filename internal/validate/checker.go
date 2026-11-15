package validate

import (
	"errors"
	"slices"
	"strings"
)

type visit int

const (
	unvisited visit = iota
	inProgress
	done
)

// checker validates extension definitions drawn from a catalog. Each definition is checked once,
// after the definitions it depends on, and a dependency cycle is reported rather than followed.
type checker struct {
	catalog *catalog
	states  map[Reference]visit
	issues  *report
}

func newChecker(loaded *catalog, issues *report) *checker {
	return &checker{catalog: loaded, states: map[Reference]visit{}, issues: issues}
}

// validateNode validates the extension id and every extension it depends on.
func (c *checker) validateNode(id Reference) {
	if resolved, err := c.catalog.lookup(id); err == nil {
		id = resolved
	} else if errors.Is(err, errAmbiguous) {
		c.issues.about(id.String(), "%v", err)
		return
	}
	switch c.states[id] {
	case inProgress:
		c.issues.about(id.String(), "extension dependency cycle includes %s", id)
		return
	case done:
		return
	}
	found := c.catalog.nodes[id]
	if found == nil {
		c.issues.about(id.String(), "missing extension definition for dependency %s", id)
		return
	}
	c.states[id] = inProgress
	for _, dependency := range found.def.Spec.Dependencies {
		c.validateNode(dependency)
	}
	c.validateDefinition(found)
	c.validateVocabulary(found, c.catalog.resolve(id))
	c.states[id] = done
}

// validateDefinition checks a definition against itself: required metadata, resolvable
// dependencies, and no name declared twice.
func (c *checker) validateDefinition(n *node) {
	def := n.def
	if def.APIVersion != "runtimeconditions.io/v1alpha1" {
		c.issues.about(n.path, "apiVersion must be runtimeconditions.io/v1alpha1")
	}
	if def.Kind != "RuntimeConditionsExtensionDefinition" {
		c.issues.about(n.path, "kind must be RuntimeConditionsExtensionDefinition")
	}
	if !def.reference().valid() {
		c.issues.about(n.path, "metadata.id is required; metadata.version is optional")
	}
	if n.id != def.reference() {
		c.issues.about(n.path, "extension id %s does not match metadata", n.id)
	}
	spec := def.Spec
	if len(spec.Dependencies) == 0 && len(spec.Kinds) == 0 && len(spec.InterfaceTypes) == 0 && len(spec.ConditionFields) == 0 &&
		len(spec.InterfaceFields) == 0 && len(spec.FieldValues) == 0 && len(spec.Schemas) == 0 {
		c.issues.about(n.path, "spec must define at least one vocabulary item or schema")
	}
	for _, dependency := range spec.Dependencies {
		if _, err := c.catalog.lookup(dependency); errors.Is(err, errAmbiguous) {
			c.issues.about(n.path, "dependency %s is ambiguous: %v", dependency, err)
		} else if err != nil {
			c.issues.about(n.path, "dependency %s cannot be resolved", dependency)
		}
	}
	c.checkSelfDuplicates(n)
}

// checkSelfDuplicates reports vocabulary a single definition declares twice, and entries that lack
// what they need.
func (c *checker) checkSelfDuplicates(n *node) {
	seen := map[string]bool{}
	check := func(key string) {
		if seen[key] {
			c.issues.about(n.path, "duplicate vocabulary definition %s", key)
		}
		seen[key] = true
	}
	spec := n.def.Spec
	for _, kind := range spec.Kinds {
		if kind.Name == "" {
			c.issues.about(n.path, "kind name is required")
			continue
		}
		check("kind:" + kind.Name)
	}
	for _, item := range spec.InterfaceTypes {
		if item.Name == "" || item.TargetKind == "" {
			c.issues.about(n.path, "interfaceTypes entries require name and targetKind")
			continue
		}
		check("interfaceType:" + item.TargetKind + ":" + item.Name)
	}
	for _, item := range spec.ConditionFields {
		if item.Name == "" {
			c.issues.about(n.path, "conditionFields entries require name")
			continue
		}
		if len(item.AppliesToKinds) == 0 {
			c.issues.about(n.path, "condition field %s must declare appliesToKinds", item.Name)
		}
		check("conditionField:" + item.Name + ":" + strings.Join(item.AppliesToKinds, ",") + ":" + strings.Join(item.AppliesToInterfaceTypes, ","))
	}
	for _, item := range spec.InterfaceFields {
		if item.Name == "" || item.TargetKind == "" || item.TargetType == "" {
			c.issues.about(n.path, "interfaceFields entries require name, targetKind, and targetType")
			continue
		}
		check("interfaceField:" + item.TargetKind + ":" + item.TargetType + ":" + item.Name)
	}
	for _, item := range spec.FieldValues {
		if item.Field == "" || item.TargetKind == "" || len(item.Values) == 0 {
			c.issues.about(n.path, "fieldValues entries require field, targetKind, and values")
			continue
		}
		check("fieldValue:" + item.TargetKind + ":" + item.TargetType + ":" + item.Field)
		valueSeen := map[string]bool{}
		for _, value := range item.Values {
			if valueSeen[value] {
				c.issues.about(n.path, "duplicate field value %q for %s", value, item.Field)
			}
			valueSeen[value] = true
		}
	}
	for _, schema := range spec.Schemas {
		if schema.ID == "" {
			c.issues.about(n.path, "schema id is required")
		}
		if schema.Description == "" {
			c.issues.about(n.path, "schema %s description is required", schema.ID)
		}
		if schema.Schema == nil {
			c.issues.about(n.path, "schema %s schema object is required", schema.ID)
		} else if _, err := compileSchema(schema.Schema); err != nil {
			c.issues.about(n.path, "schema %s is invalid: %v", schema.ID, err)
		}
		check("schema:" + schema.ID)
	}
}

// validateVocabulary checks that everything a definition references resolves to exactly one
// declaration across its dependency closure.
func (c *checker) validateVocabulary(n *node, resolved vocabulary) {
	path := n.path
	spec := n.def.Spec
	for _, item := range spec.InterfaceTypes {
		c.issues.expectExactlyOne(path, resolved.kindCount(item.TargetKind), "interface type %s targetKind %s", item.Name, item.TargetKind)
	}
	for _, item := range spec.InterfaceFields {
		c.issues.expectExactlyOne(path, resolved.kindCount(item.TargetKind), "interface field %s targetKind %s", item.Name, item.TargetKind)
		c.issues.expectExactlyOne(path, resolved.interfaceTypeCount(item.TargetKind, item.TargetType), "interface field %s targetType %s/%s", item.Name, item.TargetKind, item.TargetType)
	}
	for _, item := range spec.ConditionFields {
		for _, kind := range item.AppliesToKinds {
			c.issues.expectExactlyOne(path, resolved.kindCount(kind), "condition field %s appliesToKind %s", item.Name, kind)
			for _, targetType := range item.AppliesToInterfaceTypes {
				c.issues.expectExactlyOne(path, resolved.interfaceTypeCount(kind, targetType), "condition field %s appliesToInterfaceType %s/%s", item.Name, kind, targetType)
			}
		}
	}
	for _, item := range spec.FieldValues {
		c.issues.expectExactlyOne(path, resolved.kindCount(item.TargetKind), "field values %s targetKind %s", item.Field, item.TargetKind)
		if item.TargetType != "" {
			c.issues.expectExactlyOne(path, resolved.interfaceTypeCount(item.TargetKind, item.TargetType), "field values %s targetType %s/%s", item.Field, item.TargetKind, item.TargetType)
		}
		c.validateFieldValuePath(path, resolved, item)
	}
	for _, schema := range spec.Schemas {
		if schema.AppliesToKind != "" {
			c.issues.expectExactlyOne(path, resolved.kindCount(schema.AppliesToKind), "schema %s appliesToKind %s", schema.ID, schema.AppliesToKind)
		}
		if schema.AppliesToKind != "" && schema.AppliesToInterfaceType != "" {
			c.issues.expectExactlyOne(path, resolved.interfaceTypeCount(schema.AppliesToKind, schema.AppliesToInterfaceType), "schema %s appliesToInterfaceType %s/%s", schema.ID, schema.AppliesToKind, schema.AppliesToInterfaceType)
		}
	}
	c.checkResolvedConflicts(path, resolved)
}

// validateFieldValuePath resolves the field a fieldValues entry enumerates. A path that starts with
// interface. names an interface field. Any other path starts with the name of a condition field.
func (c *checker) validateFieldValuePath(path string, resolved vocabulary, item FieldValue) {
	switch {
	case strings.HasPrefix(item.Field, "interface."):
		field := firstPathSegment(strings.TrimPrefix(item.Field, "interface."))
		if field == "type" {
			return
		}
		c.issues.expectExactlyOne(path, resolved.interfaceFieldCount(item.TargetKind, item.TargetType, field), "fieldValues field %s interface field %s/%s/%s", item.Field, item.TargetKind, item.TargetType, field)
	default:
		field := firstPathSegment(item.Field)
		c.issues.expectExactlyOne(path, resolved.conditionFieldCount(item.TargetKind, item.TargetType, field), "fieldValues field %s condition field %s for %s/%s", item.Field, field, item.TargetKind, item.TargetType)
	}
}

// checkResolvedConflicts reports vocabulary that two extensions in the resolved set both define.
func (c *checker) checkResolvedConflicts(path string, resolved vocabulary) {
	counts := resolved.counts()
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if counts[key] > 1 {
			c.issues.about(path, "resolved extension set contains vocabulary conflict for %s", key)
		}
	}
	for _, conflict := range resolved.conditionFieldConflicts() {
		c.issues.about(path, "resolved extension set contains vocabulary conflict for %s", conflict)
	}
}

// firstPathSegment returns the leading segment of a dotted field path, dropping any list suffix.
func firstPathSegment(path string) string {
	path = strings.TrimSuffix(path, "[]")
	if index := strings.Index(path, "."); index >= 0 {
		return strings.TrimSuffix(path[:index], "[]")
	}
	return strings.TrimSuffix(path, "[]")
}
