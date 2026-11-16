package validate

import (
	"fmt"
	"slices"
)

// vocabulary is what the extensions of a resolved set define between them. A lookup counts matches
// rather than returning them, because a resolved set must define each name exactly once.
type vocabulary struct{ nodes []*node }

func (v vocabulary) kindCount(name string) int {
	count := 0
	for _, n := range v.nodes {
		for _, item := range n.def.Spec.Kinds {
			if item.Name == name {
				count++
			}
		}
	}
	return count
}

func (v vocabulary) interfaceTypeCount(kind, name string) int {
	count := 0
	for _, n := range v.nodes {
		for _, item := range n.def.Spec.InterfaceTypes {
			if item.TargetKind == kind && item.Name == name {
				count++
			}
		}
	}
	return count
}

// interfaceFieldCount counts the extensions that declare the named interface field for a kind and interface type.
func (v vocabulary) interfaceFieldCount(kind, interfaceType, name string) int {
	count := 0
	for _, n := range v.nodes {
		for _, item := range n.def.Spec.InterfaceFields {
			if item.TargetKind == kind && item.TargetType == interfaceType && item.Name == name {
				count++
			}
		}
	}
	return count
}

// conditionFieldCount counts the extensions that declare the named condition field for a kind and interface type.
func (v vocabulary) conditionFieldCount(kind, interfaceType, name string) int {
	count := 0
	for _, n := range v.nodes {
		for _, item := range n.def.Spec.ConditionFields {
			if item.Name == name && conditionFieldApplies(item, kind, interfaceType) {
				count++
			}
		}
	}
	return count
}

// fieldValueCount counts the extensions that allow value for the field path on a kind and interface type.
func (v vocabulary) fieldValueCount(field, kind, interfaceType, value string) int {
	count := 0
	for _, n := range v.nodes {
		for _, item := range n.def.Spec.FieldValues {
			if item.Field == field && item.TargetKind == kind && item.TargetType == interfaceType && slices.Contains(item.Values, value) {
				count++
			}
		}
	}
	return count
}

// interfaceFieldDeclared reports whether any extension declares the named interface field for any kind and type.
func (v vocabulary) interfaceFieldDeclared(name string) bool {
	for _, n := range v.nodes {
		for _, item := range n.def.Spec.InterfaceFields {
			if item.Name == name {
				return true
			}
		}
	}
	return false
}

// fieldValueFields returns the field paths whose values the extensions enumerate on a kind and interface type.
func (v vocabulary) fieldValueFields(kind, interfaceType string) []string {
	var fields []string
	for _, n := range v.nodes {
		for _, item := range n.def.Spec.FieldValues {
			if item.TargetKind == kind && item.TargetType == interfaceType && !slices.Contains(fields, item.Field) {
				fields = append(fields, item.Field)
			}
		}
	}
	slices.Sort(fields)
	return fields
}

// counts returns how often each vocabulary name is declared across the set. Above one is a conflict.
func (v vocabulary) counts() map[string]int {
	counts := map[string]int{}
	for _, n := range v.nodes {
		for _, item := range n.def.Spec.Kinds {
			counts["kind:"+item.Name]++
		}
		for _, item := range n.def.Spec.InterfaceTypes {
			counts["interfaceType:"+item.TargetKind+":"+item.Name]++
		}
		for _, item := range n.def.Spec.InterfaceFields {
			counts["interfaceField:"+item.TargetKind+":"+item.TargetType+":"+item.Name]++
		}
		for _, item := range n.def.Spec.FieldValues {
			counts["fieldValues:"+item.TargetKind+":"+item.TargetType+":"+item.Field]++
		}
	}
	return counts
}

// conditionFieldConflicts returns the condition fields that two extensions define over overlapping
// scopes. They cannot be counted by name alone: two definitions collide only when the kinds and
// interface types they apply to intersect.
func (v vocabulary) conditionFieldConflicts() []string {
	type definition struct {
		n     *node
		field ConditionField
	}
	var definitions []definition
	for _, n := range v.nodes {
		for _, field := range n.def.Spec.ConditionFields {
			definitions = append(definitions, definition{n, field})
		}
	}
	var conflicts []string
	for i := range definitions {
		for j := i + 1; j < len(definitions); j++ {
			left, right := definitions[i], definitions[j]
			if left.field.Name == right.field.Name && conditionFieldScopesOverlap(left.field, right.field) {
				conflicts = append(conflicts, fmt.Sprintf("conditionField:%s between %s and %s", left.field.Name, left.n.id, right.n.id))
			}
		}
	}
	return conflicts
}

func conditionFieldScopesOverlap(left, right ConditionField) bool {
	for _, kind := range left.AppliesToKinds {
		if slices.Contains(right.AppliesToKinds, kind) {
			return setsOverlapOrEitherEmpty(left.AppliesToInterfaceTypes, right.AppliesToInterfaceTypes)
		}
	}
	return false
}

// setsOverlapOrEitherEmpty treats an empty set as unrestricted, so a field scoped to every
// interface type overlaps one scoped to a specific type.
func setsOverlapOrEitherEmpty(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	for _, item := range left {
		if slices.Contains(right, item) {
			return true
		}
	}
	return false
}

func conditionFieldApplies(field ConditionField, kind, interfaceType string) bool {
	if !slices.Contains(field.AppliesToKinds, kind) {
		return false
	}
	return len(field.AppliesToInterfaceTypes) == 0 || slices.Contains(field.AppliesToInterfaceTypes, interfaceType)
}
