package validate

import (
	"errors"
	"fmt"
	"slices"
)

// profileDocument holds the profile fields the core specification defines. A condition is not
// modelled: its shape beyond kind and interface.type belongs to the extensions the profile declares.
type profileDocument struct {
	Extensions []Reference `yaml:"extensions"`
}

// The fields the core profile schema defines. Every other field of a condition comes from an extension.
var (
	coreConditionFields = map[string]bool{"name": true, "optional": true, "kind": true, "extension": true, "interface": true}
	coreInterfaceFields = map[string]bool{"type": true}
)

// profileChecker validates a profile against the vocabulary its declared extensions resolve to.
type profileChecker struct {
	checker *checker
	issues  *report
}

// validate checks the extensions a profile declares, their dependency closure, and each of its
// conditions. The core fields of the profile are checked against the core schema. conditions holds them as decoded YAML and they are never decoded into fixed types,
// because everything in a condition beyond its kind and interface type is defined by the extensions.
func (p *profileChecker) validate(profile profileDocument, conditions []any) {
	// A profile entry may be a short <url>:<version> reference or an id without a version. Look
	// each one up as the definition it names.
	catalog := p.checker.catalog
	extensions := make([]Reference, len(profile.Extensions))
	for i, id := range profile.Extensions {
		extensions[i] = id
		if resolved, err := catalog.lookup(id); err == nil {
			extensions[i] = resolved
		} else if errors.Is(err, errAmbiguous) {
			p.issues.addf("%v", err)
		}
	}

	declared := map[Reference]bool{}
	for _, id := range extensions {
		if declared[id] {
			p.issues.addf("duplicate extension id %s", id)
			continue
		}
		declared[id] = true
		if catalog.nodes[id] == nil {
			p.issues.addf("missing extension definition for %s", id)
			continue
		}
		p.checker.validateNode(id)
	}

	// A profile must name every extension it depends on, not just those it uses directly, so that
	// its vocabulary can be rebuilt from the list alone.
	closure := p.closure(extensions)
	for id := range closure {
		if !declared[id] {
			p.issues.addf("extensions missing dependency %s", id)
		}
	}

	resolved := p.vocabulary(closure)
	p.checker.checkResolvedConflicts("profile", resolved)
	for index, condition := range conditions {
		p.validateCondition(index, resolved, condition)
	}
}

func (p *profileChecker) closure(ids []Reference) map[Reference]bool {
	seen := map[Reference]bool{}
	var visit func(Reference)
	visit = func(id Reference) {
		if seen[id] {
			return
		}
		seen[id] = true
		found := p.checker.catalog.nodes[id]
		if found == nil {
			return
		}
		for _, dependency := range found.def.Spec.Dependencies {
			if resolved, err := p.checker.catalog.lookup(dependency); err == nil {
				dependency = resolved
			}
			visit(dependency)
		}
	}
	for _, id := range ids {
		visit(id)
	}
	return seen
}

func (p *profileChecker) vocabulary(ids map[Reference]bool) vocabulary {
	var nodes []*node
	for id := range ids {
		if found := p.checker.catalog.nodes[id]; found != nil {
			nodes = append(nodes, found)
		}
	}
	slices.SortFunc(nodes, func(left, right *node) int { return left.id.compare(right.id) })
	return vocabulary{nodes: nodes}
}

// validateCondition checks one condition, held as decoded YAML, against the vocabulary and schemas
// of the extensions the profile declares. It knows only what the core schema defines.
func (p *profileChecker) validateCondition(index int, resolved vocabulary, raw any) {
	prefix := fmt.Sprintf("conditions[%d]", index)
	condition, ok := raw.(map[string]any)
	if !ok {
		p.issues.addf("%s must be a mapping", prefix)
		return
	}
	kind, _ := condition["kind"].(string)
	interfaceFields, _ := condition["interface"].(map[string]any)
	interfaceType, _ := interfaceFields["type"].(string)
	p.issues.expectExactlyOne("", resolved.kindCount(kind), "%s.kind %s", prefix, kind)
	p.issues.expectExactlyOne("", resolved.interfaceTypeCount(kind, interfaceType), "%s.interface.type %s/%s", prefix, kind, interfaceType)

	// Every field of a condition beyond the core ones belongs to an extension, so it has to be
	// defined for this kind and interface type by one of the profile's extensions. That also catches
	// a field used without declaring the extension that defines it.
	for _, name := range sortedKeys(condition) {
		if !coreConditionFields[name] {
			p.issues.expectExactlyOne("", resolved.conditionFieldCount(kind, interfaceType, name), "%s.%s for %s/%s", prefix, name, kind, interfaceType)
		}
	}
	// An interface field that some extension defines has to be defined for this kind and interface
	// type. An interface field no extension defines is left to the extension schemas, since an
	// extension may allow extra properties there.
	for _, name := range sortedKeys(interfaceFields) {
		if !coreInterfaceFields[name] && resolved.interfaceFieldDeclared(name) {
			p.issues.expectExactlyOne("", resolved.interfaceFieldCount(kind, interfaceType, name), "%s.interface.%s for %s/%s", prefix, name, kind, interfaceType)
		}
	}

	// Where extensions enumerate the values of a field path, every string the condition holds there
	// has to be one of them. A value that is not a string is for the extension's schema to judge.
	for _, field := range resolved.fieldValueFields(kind, interfaceType) {
		for _, found := range valuesAt(condition, field) {
			if value, isString := found.value.(string); isString {
				p.issues.expectExactlyOne("", resolved.fieldValueCount(field, kind, interfaceType, value), "%s.%s %s for %s/%s", prefix, found.path, value, kind, interfaceType)
			}
		}
	}
	p.validateSchemas(prefix, resolved, kind, interfaceType, raw)
}

// validateSchemas applies every extension schema whose scope matches the condition. A schema with
// no declared scope applies to all conditions.
func (p *profileChecker) validateSchemas(prefix string, resolved vocabulary, kind, interfaceType string, raw any) {
	for _, n := range resolved.nodes {
		for _, item := range n.def.Spec.Schemas {
			if item.AppliesToKind != "" && item.AppliesToKind != kind {
				continue
			}
			if item.AppliesToInterfaceType != "" && item.AppliesToInterfaceType != interfaceType {
				continue
			}
			schema, err := compileSchema(item.Schema)
			if err != nil {
				p.issues.addf("%s schema %s could not be compiled: %v", prefix, item.ID, err)
				continue
			}
			instance, err := jsonValue(raw)
			if err != nil {
				p.issues.addf("%s could not be converted for schema validation: %v", prefix, err)
				continue
			}
			if err := schema.Validate(instance); err != nil {
				p.issues.addf("%s does not satisfy extension schema %s: %v", prefix, item.ID, err)
			}
		}
	}
}
