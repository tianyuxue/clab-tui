package containerlab

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
)

type kindManifest struct {
	Kinds []kindCase `yaml:"kinds"`
}

// kindCase is the complete manifest schema implemented by this matrix.
// Optional profiles are supported by the loader, but the checked-in matrix
// contains only verified license-free cases.
type kindCase struct {
	Name          string `yaml:"name"`
	Image         string `yaml:"image"`
	Profile       string `yaml:"profile"`
	LicenseFree   bool   `yaml:"license_free"`
	Interface     string `yaml:"interface"`
	TimeoutSecond int    `yaml:"timeout_seconds"`
}

func loadKindManifest(path string) ([]kindCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read kind manifest: %w", err)
	}
	var manifest kindManifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse kind manifest: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parse kind manifest: trailing YAML document")
		}
		return nil, fmt.Errorf("parse kind manifest trailing document: %w", err)
	}
	if len(manifest.Kinds) == 0 {
		return nil, fmt.Errorf("kind manifest has no kinds")
	}
	seen := make(map[string]struct{}, len(manifest.Kinds))
	for i, item := range manifest.Kinds {
		if strings.TrimSpace(item.Name) == "" {
			return nil, fmt.Errorf("kind %d has empty name", i)
		}
		if _, ok := seen[item.Name]; ok {
			return nil, fmt.Errorf("duplicate kind name %q", item.Name)
		}
		seen[item.Name] = struct{}{}
		if strings.TrimSpace(item.Image) == "" {
			return nil, fmt.Errorf("kind %q has empty image", item.Name)
		}
		if strings.TrimSpace(item.Interface) == "" {
			return nil, fmt.Errorf("kind %q has empty interface", item.Name)
		}
		if item.LicenseFree && item.Profile == "" {
			return nil, fmt.Errorf("kind %q marks license_free without profile", item.Name)
		}
		if !item.LicenseFree {
			return nil, fmt.Errorf("kind %q is not verified license-free", item.Name)
		}
		if item.Profile != "required" && item.Profile != "optional" {
			return nil, fmt.Errorf("kind %q has invalid profile %q", item.Name, item.Profile)
		}
		if item.TimeoutSecond <= 0 {
			return nil, fmt.Errorf("kind %q has invalid timeout_seconds %d", item.Name, item.TimeoutSecond)
		}
	}

	filter := strings.TrimSpace(os.Getenv("CLAB_TUI_KIND_FILTER"))
	allowed := make(map[string]struct{})
	if filter != "" {
		for _, name := range strings.Split(filter, ",") {
			if name = strings.TrimSpace(name); name != "" {
				allowed[name] = struct{}{}
			}
		}
	}
	optionalEnabled := envEnabled("CLAB_TUI_ENABLE_OPTIONAL_KINDS")
	selected := make([]kindCase, 0, len(manifest.Kinds))
	for _, item := range manifest.Kinds {
		if filter != "" {
			if _, ok := allowed[item.Name]; !ok {
				continue
			}
		}
		if item.Profile == "optional" && !optionalEnabled {
			continue
		}
		selected = append(selected, item)
	}
	return selected, nil
}

func envEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func TestKindManifestLoadsVerifiedCases(t *testing.T) {
	path := writeKindManifest(t, `kinds:
  - name: linux
    image: alpine:3.20
    profile: required
    license_free: true
    interface: eth1
    timeout_seconds: 90
  - name: optional
    image: example:latest
    profile: optional
    license_free: true
    interface: eth1
    timeout_seconds: 10
`)

	t.Setenv("CLAB_TUI_ENABLE_OPTIONAL_KINDS", "1")
	cases, err := loadKindManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].Name != "linux" || cases[1].Profile != "optional" {
		t.Fatalf("cases = %#v", cases)
	}
}

func TestKindManifestRejectsInvalidRecords(t *testing.T) {
	tests := map[string]string{
		"duplicate name": `kinds:
  - name: linux
    image: alpine:3.20
    profile: required
    license_free: true
    interface: eth1
    timeout_seconds: 90
  - name: linux
    image: alpine:3.20
    profile: required
    license_free: true
    interface: eth1
    timeout_seconds: 90
`,
		"empty image": `kinds:
  - name: linux
    image: ""
    profile: required
    license_free: true
    interface: eth1
    timeout_seconds: 90
`,
		"empty interface": `kinds:
  - name: linux
    image: alpine:3.20
    profile: required
    license_free: true
    interface: ""
    timeout_seconds: 90
`,
		"license-free without profile": `kinds:
  - name: linux
    image: alpine:3.20
    license_free: true
    interface: eth1
    timeout_seconds: 90
`,
		"not verified license-free": `kinds:
  - name: linux
    image: alpine:3.20
    profile: required
    license_free: false
    interface: eth1
    timeout_seconds: 90
`,
		"trailing document": `kinds:
  - name: linux
    image: alpine:3.20
    profile: required
    license_free: true
    interface: eth1
    timeout_seconds: 90
---
`,
		"malformed trailing document": `kinds:
  - name: linux
    image: alpine:3.20
    profile: required
    license_free: true
    interface: eth1
    timeout_seconds: 90
---
`,
	}
	for name, manifest := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := loadKindManifest(writeKindManifest(t, manifest)); err == nil {
				t.Fatal("loadKindManifest succeeded")
			}
		})
	}
}

func TestKindManifestFilter(t *testing.T) {
	path := writeKindManifest(t, `kinds:
  - name: linux
    image: alpine:3.20
    profile: required
    license_free: true
    interface: eth1
    timeout_seconds: 90
  - name: srlinux
    image: srlinux:latest
    profile: required
    license_free: true
    interface: e1-1
    timeout_seconds: 90
`)
	t.Setenv("CLAB_TUI_KIND_FILTER", " srlinux ")
	cases, err := loadKindManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Name != "srlinux" {
		t.Fatalf("filtered cases = %#v", cases)
	}
}

func TestKindManifestOptionalKindsAreGated(t *testing.T) {
	path := writeKindManifest(t, `kinds:
  - name: optional
    image: example:latest
    profile: optional
    license_free: true
    interface: eth1
    timeout_seconds: 10
`)
	cases, err := loadKindManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Fatalf("optional cases without gate = %#v", cases)
	}
}

func TestKindMatrixOptionalFailureClassification(t *testing.T) {
	tests := map[string]bool{
		"no such image: example:latest":                                      true,
		"pull access denied for example:latest":                              true,
		"containerlab requires root privileges or root via SUID":             true,
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock": true,
		"operation not permitted while creating network namespace":           true,
		"deploy exited 1: topology validation failed":                        false,
		"deploy exited 1: malformed topology":                                false,
		"lab did not converge within 90s":                                    false,
		"required interface e1-1 was not observed":                           false,
	}
	for message, wantSkip := range tests {
		if got := optionalKindFailureIsUnavailable(message); got != wantSkip {
			t.Errorf("optionalKindFailureIsUnavailable(%q) = %t, want %t", message, got, wantSkip)
		}
	}
}

func TestKindMatrixRequiredBinarySelection(t *testing.T) {
	if kindManifestHasRequired([]kindCase{{Profile: "optional"}}) {
		t.Fatal("optional-only manifest requires strict binary preflight")
	}
	if kindManifestHasRequired([]kindCase{{Profile: "optional"}, {Profile: "required"}}) == false {
		t.Fatal("manifest with required case did not require strict binary preflight")
	}
}

func TestKindMatrixLabNamesAreUniqueAndSafe(t *testing.T) {
	first := kindMatrixLabName("nokia_srlinux")
	second := kindMatrixLabName("nokia_srlinux")
	if first == second {
		t.Fatalf("lab names are not unique: %q", first)
	}
	if strings.Trim(first, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		t.Fatalf("lab name contains unsafe characters: %q", first)
	}
}

var kindMatrixNameCounter atomic.Uint64

func kindMatrixLabName(kind string) string {
	var safe strings.Builder
	for _, r := range strings.ToLower(kind) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			safe.WriteRune(r)
		} else {
			safe.WriteByte('-')
		}
	}
	return fmt.Sprintf("clab-tui-kind-%s-%d-%d", safe.String(), os.Getpid(), kindMatrixNameCounter.Add(1))
}

func kindManifestHasRequired(cases []kindCase) bool {
	for _, item := range cases {
		if item.Profile == "required" {
			return true
		}
	}
	return false
}

func optionalKindFailureIsUnavailable(message string) bool {
	message = strings.ToLower(message)
	for _, marker := range []string{
		"no such image",
		"pull access denied",
		"requires root privileges",
		"cannot connect to the docker daemon",
		"docker daemon unavailable",
		"operation not permitted",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func writeKindManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kinds.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
