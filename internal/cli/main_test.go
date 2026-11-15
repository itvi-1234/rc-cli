package cli

import (
	"os"
	"testing"

	"github.com/runtimeconditions/rc-cli/internal/validate"
)

func TestMain(m *testing.M) {
	// The command is tested against a made-up core schema, not the version the real build embeds.
	schema, err := os.ReadFile("../validate/testdata/core.schema.yaml")
	if err != nil {
		panic(err)
	}
	validate.UseCoreSchema(schema)
	os.Exit(m.Run())
}
