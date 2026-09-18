//go:build integration

package containerlab

import "testing"

func TestTraceIntegrationImageIsInterfaceCapable(t *testing.T) {
	if traceIntegrationImage != "nicolaka/netshoot:v0.13" {
		t.Fatalf("trace integration image = %q, want pinned interface-capable netshoot image", traceIntegrationImage)
	}
}
