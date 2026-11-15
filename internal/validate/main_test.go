package validate

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	schema, err := os.ReadFile("testdata/core.schema.yaml")
	if err != nil {
		panic(err)
	}
	UseCoreSchema(schema)
	os.Exit(m.Run())
}
