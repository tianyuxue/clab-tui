//go:build integration

package containerlab

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tianyuxue/clab-tui/internal/engine"
)

func TestIntegrationKindMatrix(t *testing.T) {
	if !integrationEnabled(t) {
		t.Skip("CLAB_TUI_SKIP_INTEGRATION set")
	}
	manifest := filepath.Join(kindMatrixRepoRoot(t), "testdata", "integration", "kinds.yaml")
	cases, err := loadKindManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Skip("kind filter selected no cases")
	}
	bin := ""
	if kindManifestHasRequired(cases) {
		bin = requireRequiredContainerlab(t)
	} else {
		bin = integrationBinary(t)
	}

	for _, item := range cases {
		item := item
		t.Run(item.Name, func(t *testing.T) {
			if item.Profile == "required" {
				requireRequiredKindImage(t, item)
			} else if reason := kindImageUnavailableReason(item.Image); reason != "" {
				t.Skipf("optional kind %q skipped: %s", item.Name, reason)
			}

			dir := t.TempDir()
			lab := kindMatrixLabName(item.Name)
			topo := filepath.Join(dir, lab+".clab.yml")
			// Two nodes are linked so the declared business interface actually
			// exists: containerlab only creates non-eth0 interfaces from links,
			// a single-node lab has none.
			content := fmt.Sprintf("name: %s\ntopology:\n  nodes:\n    node1:\n      kind: %s\n      image: %s\n    node2:\n      kind: %s\n      image: %s\n  links:\n    - endpoints: [\"node1:%s\", \"node2:%s\"]\n",
				lab, item.Name, item.Image, item.Name, item.Image, item.Interface, item.Interface)
			if err := os.WriteFile(topo, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}

			e, err := New(WithBinary(bin), WithLabDir(dir))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = e.Close() })
			cleanupSucceeded := false
			t.Cleanup(func() {
				if cleanupSucceeded {
					return
				}
				destroyLab(t, e, lab)
				assertNoLabResidue(t, lab)
			})

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(item.TimeoutSecond)*time.Second)
			defer cancel()
			ch, err := e.Deploy(ctx, topo)
			if err != nil {
				if item.Profile == "optional" && optionalKindFailureIsUnavailable(err.Error()) {
					t.Skipf("optional kind %q skipped: unavailable image or containerlab capability: %v", item.Name, err)
				}
				t.Fatalf("kind %q deploy: %v", item.Name, err)
			}
			output, code, outputErr := collectOutputBestEffort(t, ctx, ch)
			if outputErr != nil {
				if item.Profile == "optional" && optionalKindFailureIsUnavailable(output+"\n"+outputErr.Error()) {
					t.Skipf("optional kind %q skipped: image or containerlab capability unavailable: %v", item.Name, outputErr)
				}
				t.Fatalf("kind %q deploy output: %v\n%s", item.Name, outputErr, output)
			}
			if code != 0 {
				if item.Profile == "optional" && optionalKindFailureIsUnavailable(output) {
					t.Skipf("optional kind %q skipped: unavailable image or containerlab capability (deploy exit %d)", item.Name, code)
				}
				t.Fatalf("kind %q deploy exited %d:\n%s", item.Name, code, output)
			}
			if labs, err := e.ListLabs(ctx); err != nil {
				t.Fatalf("ListLabs after deploying required kind %q: %v", item.Name, err)
			} else if !hasLab(labs, lab) {
				t.Fatalf("ListLabs after deploying required kind %q did not contain %q", item.Name, lab)
			}

			waitForLab(t, e, lab, time.Duration(item.TimeoutSecond)*time.Second, func(current *engine.Lab) bool {
				if len(current.Nodes) != 2 {
					return false
				}
				var node1 *engine.Node
				for i := range current.Nodes {
					if current.Nodes[i].Name == "node1" {
						node1 = &current.Nodes[i]
					}
				}
				if node1 == nil || node1.State != engine.StatusRunning {
					return false
				}
				for _, iface := range node1.Interfaces {
					if iface.Name == item.Interface {
						return true
					}
				}
				return false
			})
			strictDestroyLab(t, e, lab)
			assertNoLabResidue(t, lab)
			cleanupSucceeded = true
		})
	}
}

func requireRequiredContainerlab(t *testing.T) string {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Fatalf("required kind matrix cannot run: root privileges are required for containerlab integration")
	}
	bin := os.Getenv("CLAB_BIN")
	if bin == "" {
		bin = "containerlab"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Fatalf("required kind matrix cannot run: containerlab binary %q unavailable: %v", bin, err)
	}
	return bin
}

func requireRequiredKindImage(t *testing.T, item kindCase) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("required kind %q cannot run: command \"docker\" unavailable: %v", item.Name, err)
	}
	if output, err := runDockerCommand("info"); err != nil {
		t.Fatalf("required kind %q cannot run: Docker daemon unavailable: %v: %s", item.Name, err, strings.TrimSpace(string(output)))
	}
	if output, err := runDockerCommand("image", "inspect", item.Image); err != nil {
		t.Fatalf("required kind %q image %q unavailable: %v: %s", item.Name, item.Image, err, strings.TrimSpace(string(output)))
	}
}

func kindImageUnavailableReason(image string) string {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Sprintf("command \"docker\" unavailable: %v", err)
	}
	if output, err := runDockerCommand("info"); err != nil {
		return fmt.Sprintf("Docker daemon unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := runDockerCommand("image", "inspect", image); err != nil {
		return fmt.Sprintf("Docker image %q unavailable: %v: %s", image, err, strings.TrimSpace(string(output)))
	}
	return ""
}

func hasLab(labs []*engine.Lab, name string) bool {
	for _, lab := range labs {
		if lab != nil && lab.Name == name {
			return true
		}
	}
	return false
}

func strictDestroyLab(t *testing.T, e *ClabEngine, lab string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ch, err := e.Destroy(ctx, lab, engine.WithDestroyCleanup())
	if err != nil {
		t.Fatalf("destroy %q: %v", lab, err)
	}
	output, code, outputErr := collectOutputBestEffort(t, ctx, ch)
	if outputErr != nil {
		t.Fatalf("destroy %q output: %v\n%s", lab, outputErr, output)
	}
	if code != 0 {
		t.Fatalf("destroy %q exited %d:\n%s", lab, code, output)
	}
}

func kindMatrixRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
