package validate

import (
	"bytes"
	"encoding/json"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// compileSchema compiles the JSON Schema an extension declares inline.
func compileSchema(value any) (*jsonschema.Schema, error) {
	parsed, err := jsonValue(value)
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	// An absolute identifier, so that messages do not carry the directory the command ran in.
	const location = "urn:runtimeconditions:extension-schema"
	if err := compiler.AddResource(location, parsed); err != nil {
		return nil, err
	}
	return compiler.Compile(location)
}

// jsonValue converts a YAML-decoded value into the JSON representation the schema library expects.
func jsonValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(data))
}
