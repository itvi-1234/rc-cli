package validate

import (
	"go.yaml.in/yaml/v3"
)

// Profile validates a profile against the core profile schema and against the extension definitions
// found beneath catalogRoots. The profile is checked as data against what the extensions it
// declares define, so the result does not depend on which language or tool produced it. A nil error
// means the profile is valid.
func Profile(data []byte, catalogRoots []string) error {
	var profile profileDocument
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return err
	}
	// Conditions are decoded as free-form values and never into fixed types: what a condition may
	// hold beyond its kind and interface type is defined by the extensions the profile declares, and
	// their schemas validate the document as written.
	var raw struct {
		Conditions []any `yaml:"conditions"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	// The core profile schema covers what the core specification defines: the fields of the profile
	// itself, the shape of the extension list, and that a condition has a kind and an interface type.
	core, err := loadCoreSchema()
	if err != nil {
		return err
	}
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	instance, err := jsonValue(document)
	if err != nil {
		return err
	}
	coreIssues := &report{}
	if err := core.Validate(instance); err != nil {
		coreIssues.addf("the profile does not satisfy the core profile schema: %v", err)
	}

	issues := &report{}
	loaded, err := loadCatalog(catalogRoots, issues)
	if err != nil {
		return err
	}
	// Problems found while reading the definitions come first, then the profile's own.
	definitionIssues := issues.messages
	issues = &report{}
	(&profileChecker{checker: newChecker(loaded, issues), issues: issues}).validate(profile, raw.Conditions)
	issues.messages = append(append(coreIssues.messages, definitionIssues...), issues.messages...)
	return issues.err()
}
