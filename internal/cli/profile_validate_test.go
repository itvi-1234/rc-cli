package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The command is tested with two made-up extensions, so these tests do not depend on any real
// extension or on which version of it exists. How the validator treats extensions is tested in
// internal/validate.
const (
	alphaRef = "https://example.test/dummy/alpha:v1"
	betaRef  = "https://example.test/dummy/beta:v2"

	validProfile = `apiVersion: runtimeconditions.io/v1alpha1
kind: RuntimeConditionsProfile
metadata: {name: shapes}
workload: {uri: "https://example.test/shapes"}
extensions:
  - ` + alphaRef + `
  - ` + betaRef + `
conditions:
  - kind: widget
    interface: {type: plain, mode: fast}
  - kind: gadget
    interface: {type: strict, operations: [read, write]}
`
)

var fixtureRoot = filepath.Join("..", "..", "testdata", "profile-validate", "extensions")

// definitionServer serves the committed definitions for https://example.test/.
type definitionServer struct{ requests []string }

func (s *definitionServer) RoundTrip(request *http.Request) (*http.Response, error) {
	s.requests = append(s.requests, request.URL.String())
	status, body := http.StatusNotFound, "not found"
	const prefix = "https://example.test/"
	if address := request.URL.String(); strings.HasPrefix(address, prefix) {
		if data, err := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(strings.TrimPrefix(address, prefix)))); err == nil {
			status, body = http.StatusOK, string(data)
		}
	}
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

type refusingTransport struct{}

func (refusingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("the network must not be used")
}

func runProfileValidate(t *testing.T, transport http.RoundTripper, profile string, args ...string) (string, error) {
	t.Helper()
	api := linkedProfileValidateAPI()
	api.Client = &http.Client{Transport: transport}
	command := newProfileValidateCommand(api)
	var stderr bytes.Buffer
	command.SetOut(io.Discard)
	command.SetErr(&stderr)
	command.SetIn(strings.NewReader(profile))
	command.SetArgs(append([]string{"-"}, args...))
	err := command.Execute()
	return stderr.String(), err
}

func TestValidateAcceptsAProfileAndDownloadsItsExtensionsAndDependencies(t *testing.T) {
	server := &definitionServer{}
	stderr, err := runProfileValidate(t, server, validProfile)
	if err != nil {
		t.Fatalf("a valid profile was rejected: %v", err)
	}
	if !strings.Contains(stderr, "profile validated") {
		t.Fatalf("missing success message: %q", stderr)
	}
	if len(server.requests) < 2 {
		t.Fatalf("expected both extensions to be downloaded, got %v", server.requests)
	}
}

func TestValidateDownloadsADefinitionOnceWhenItIsNamedInTwoWays(t *testing.T) {
	// beta depends on alpha by its full id, and the profile names alpha by its short form.
	server := &definitionServer{}
	if _, err := runProfileValidate(t, server, validProfile); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, request := range server.requests {
		seen[request]++
	}
	if seen["https://example.test/dummy/alpha/v1/runtimeconditions.extension.yaml"] < 1 {
		t.Fatalf("alpha was not downloaded: %v", server.requests)
	}
}

func TestValidateRejectsAValueTheExtensionDoesNotAllow(t *testing.T) {
	_, err := runProfileValidate(t, &definitionServer{}, strings.Replace(validProfile, "mode: fast", "mode: turbo", 1))
	if err == nil || !strings.Contains(err.Error(), "turbo") {
		t.Fatalf("expected the mode to be rejected, got %v", err)
	}
}

func TestValidateRejectsAConditionWhoseExtensionIsNotDeclared(t *testing.T) {
	withoutBeta := strings.Replace(validProfile, "  - "+betaRef+"\n", "", 1)
	_, err := runProfileValidate(t, &definitionServer{}, withoutBeta)
	if err == nil || !strings.Contains(err.Error(), "kind gadget") {
		t.Fatalf("expected the gadget kind to be unknown without beta, got %v", err)
	}
}

func TestValidateSaysHowToProceedWhenADefinitionCannotBeDownloaded(t *testing.T) {
	profile := strings.Replace(validProfile, "dummy/alpha:v1", "dummy/missing:v9", 1)
	_, err := runProfileValidate(t, &definitionServer{}, profile)
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "--catalog") {
		t.Fatalf("expected a download error that mentions --catalog, got %v", err)
	}
}

func TestValidateDownloadsOnlyOverHTTPS(t *testing.T) {
	profile := strings.Replace(validProfile, "https://example.test/dummy/alpha:v1", "http://example.test/dummy/alpha:v1", 1)
	server := &definitionServer{}
	_, err := runProfileValidate(t, server, profile)
	if err == nil || !strings.Contains(err.Error(), "only https") {
		t.Fatalf("expected a plain http id to be refused, got %v", err)
	}
	for _, request := range server.requests {
		if strings.HasPrefix(request, "http://") {
			t.Fatalf("a plain http request was made: %s", request)
		}
	}
}

func TestValidateReadsLocalDefinitionsWithoutTheNetworkWhenACatalogIsGiven(t *testing.T) {
	stderr, err := runProfileValidate(t, refusingTransport{}, validProfile, "--catalog", fixtureRoot)
	if err != nil {
		t.Fatalf("a valid profile was rejected with a local catalog: %v", err)
	}
	if !strings.Contains(stderr, "profile validated") {
		t.Fatalf("missing success message: %q", stderr)
	}
}

func TestValidateReadsTheProfileFromAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, []byte(validProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	api := linkedProfileValidateAPI()
	api.Client = &http.Client{Transport: refusingTransport{}}
	command := newProfileValidateCommand(api)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.SetArgs([]string{path, "--catalog", fixtureRoot})
	if err := command.Execute(); err != nil {
		t.Fatalf("a valid profile file was rejected: %v", err)
	}
}

func TestValidateIsPartOfTheProfileCommand(t *testing.T) {
	root := NewRootCommandWith(BuildInfo{}, IOStreams{}.normalized(), DefaultDependencies())
	command, _, err := root.Find([]string{"profile", "validate"})
	if err != nil || command.Name() != "validate" {
		t.Fatalf("rc profile validate is not registered: %v %v", command, err)
	}
}
