package validate

import (
	"cmp"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// DefinitionFile is the conventional file name of an extension definition.
const DefinitionFile = "runtimeconditions.extension.yaml"

// Reference identifies an extension release. A definition and a dependency write it as an id and a
// version object. A profile also writes it as a string, either the id alone, or <short-url>:<version>
// where the short URL is the id without its trailing /<version>/runtimeconditions.extension.yaml.
type Reference struct {
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
}

func (r Reference) valid() bool    { return r.ID != "" }
func (r Reference) String() string { return r.ID + "@" + r.Version }

func (r Reference) compare(other Reference) int {
	if order := cmp.Compare(r.ID, other.ID); order != 0 {
		return order
	}
	return cmp.Compare(r.Version, other.Version)
}

// UnmarshalYAML reads the object form and the string forms.
func (r *Reference) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		var fields map[string]any
		if err := node.Decode(&fields); err != nil {
			return err
		}
		id, idOK := fields["id"].(string)
		version, versionOK := fields["version"].(string)
		if len(fields) != 2 || !idOK || id == "" || !versionOK || version == "" {
			return fmt.Errorf("extension reference must contain non-empty string id and version")
		}
		r.ID, r.Version = id, version
		return nil
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" || node.Value == "" {
		return fmt.Errorf("extension reference must be an id/version object or a non-empty string")
	}
	value := node.Value
	separator := strings.LastIndex(value, ":")
	schemeEnd := strings.Index(value, "://")
	if separator > 0 && separator > schemeEnd+2 && separator > strings.LastIndex(value, "/") {
		if separator == len(value)-1 {
			return fmt.Errorf("extension reference version suffix must be non-empty")
		}
		r.ID, r.Version = value[:separator], value[separator+1:]
	} else {
		r.ID, r.Version = value, ""
	}
	return nil
}

// expandShort turns a short <url>:<version> reference back into the full id of a definition. A
// reference that already names a definition, or has no version, is returned unchanged.
func expandShort(r Reference) Reference {
	if r.Version == "" || !strings.HasPrefix(r.ID, "https://") || strings.HasSuffix(r.ID, "/"+DefinitionFile) {
		return r
	}
	return Reference{ID: r.ID + "/" + r.Version + "/" + DefinitionFile, Version: r.Version}
}
