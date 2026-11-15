package validate

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

// coreSchemaBase64 is set by the release build from the separately versioned core profile schema,
// after its SHA-256 has been checked (scripts/build.go). A plain go build leaves it empty and
// validation fails closed, because a profile cannot be called valid without the core schema.
var coreSchemaBase64 string

var coreSchemaOverride []byte

// UseCoreSchema makes the validator use the given core profile schema instead of the one the build
// embedded. It is meant for tests and for tools that bring their own schema.
func UseCoreSchema(schema []byte) { coreSchemaOverride = schema }

var errNoCoreSchema = errors.New("this build of rc has no verified core profile schema")

// loadCoreSchema compiles the core profile schema.
func loadCoreSchema() (*jsonschema.Schema, error) {
	data := coreSchemaOverride
	if len(data) == 0 {
		if coreSchemaBase64 == "" {
			return nil, errNoCoreSchema
		}
		decoded, err := base64.StdEncoding.DecodeString(coreSchemaBase64)
		if err != nil {
			return nil, fmt.Errorf("the embedded core profile schema is not valid base64: %w", err)
		}
		data = decoded
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("read the core profile schema: %w", err)
	}
	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return nil, errors.New("the core profile schema must use JSON Schema Draft 2020-12")
	}
	identifier, _ := document["$id"].(string)
	if identifier == "" {
		return nil, errors.New("the core profile schema has no $id")
	}
	parsed, err := jsonValue(document)
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(identifier, parsed); err != nil {
		return nil, err
	}
	return compiler.Compile(identifier)
}
