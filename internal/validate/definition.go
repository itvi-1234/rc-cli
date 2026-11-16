package validate

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// Definition is a RuntimeConditionsExtensionDefinition document: the vocabulary an extension
// contributes and the extensions it builds on.
type Definition struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		ID      string `yaml:"id"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec Spec `yaml:"spec"`
}

// Spec is the vocabulary a definition contributes. A key it does not know is ignored.
type Spec struct {
	Dependencies    []Reference      `yaml:"dependencies"`
	Kinds           []Kind           `yaml:"kinds"`
	InterfaceTypes  []InterfaceType  `yaml:"interfaceTypes"`
	ConditionFields []ConditionField `yaml:"conditionFields"`
	InterfaceFields []InterfaceField `yaml:"interfaceFields"`
	FieldValues     []FieldValue     `yaml:"fieldValues"`
	Schemas         []Schema         `yaml:"schemas"`
}

// Kind declares a condition kind.
type Kind struct {
	Name string `yaml:"name"`
}

// InterfaceType declares an interface type available to a kind.
type InterfaceType struct {
	Name       string `yaml:"name"`
	TargetKind string `yaml:"targetKind"`
}

// ConditionField declares a field of the condition itself and the kinds and interface types it applies to.
type ConditionField struct {
	Name                    string   `yaml:"name"`
	AppliesToKinds          []string `yaml:"appliesToKinds"`
	AppliesToInterfaceTypes []string `yaml:"appliesToInterfaceTypes"`
}

// InterfaceField declares a field of the interface of one kind and interface type.
type InterfaceField struct {
	Name       string `yaml:"name"`
	TargetKind string `yaml:"targetKind"`
	TargetType string `yaml:"targetType"`
}

// FieldValue enumerates the values a field accepts. Field is a path such as
// interface.operations[].method or configuration.env[].property.
type FieldValue struct {
	Field      string   `yaml:"field"`
	TargetKind string   `yaml:"targetKind"`
	TargetType string   `yaml:"targetType"`
	Values     []string `yaml:"values"`
}

// Schema is a JSON Schema applied to the conditions it matches.
type Schema struct {
	ID                     string `yaml:"id"`
	Description            string `yaml:"description"`
	AppliesToKind          string `yaml:"appliesToKind"`
	AppliesToInterfaceType string `yaml:"appliesToInterfaceType"`
	Schema                 any    `yaml:"schema"`
}

func (d Definition) reference() Reference {
	if d.Metadata.ID == "" {
		return Reference{}
	}
	return Reference{ID: d.Metadata.ID, Version: d.Metadata.Version}
}

// readDefinition reads path and reports whether it holds an extension definition. A document of any
// other kind is skipped rather than rejected, since a directory of definitions may hold other YAML.
func readDefinition(path string) (Definition, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, false, err
	}
	var probe struct {
		Kind     string         `yaml:"kind"`
		Metadata map[string]any `yaml:"metadata"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return Definition{}, false, err
	}
	if probe.Kind != "RuntimeConditionsExtensionDefinition" {
		return Definition{}, false, nil
	}
	identifier, idOK := probe.Metadata["id"].(string)
	version, versionOK := probe.Metadata["version"].(string)
	_, hasVersion := probe.Metadata["version"]
	_, hasURI := probe.Metadata["uri"]
	if !idOK || identifier == "" || (hasVersion && (!versionOK || version == "")) || hasURI {
		return Definition{}, false, fmt.Errorf("metadata.id is required and metadata.version, when present, must be a non-empty string")
	}
	var definition Definition
	if err := yaml.Unmarshal(data, &definition); err != nil {
		return Definition{}, false, err
	}
	return definition, true, nil
}
