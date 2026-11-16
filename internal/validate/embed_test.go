package validate

import (
	"os"
	"strings"
	"testing"
)

// Both ways of building rc embed the core schema through linker flags: scripts/build.go for local
// and CI builds, scripts/release-package.py for the release archives and packages. The release
// build once set only the Go profiler's variable, so a released rc refused to validate. CI builds
// with build.go only, so nothing else would notice that again.
func TestEveryBuildEmbedsTheCoreSchemaInTheValidator(t *testing.T) {
	const variable = "github.com/runtimeconditions/rc-cli/internal/validate.coreSchemaBase64"
	for _, script := range []string{"../../scripts/build.go", "../../scripts/release-package.py"} {
		data, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), variable) {
			t.Errorf("%s does not set %s, so a build made with it cannot validate profiles", script, variable)
		}
	}
}
