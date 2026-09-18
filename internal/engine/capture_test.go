package engine

import (
	"context"
	"testing"
)

func TestPacketTypes(t *testing.T) {
	ev := PacketEvent{Node: "r1", Iface: "e1-1", Pkt: ParsedPacket{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 179, DstPort: 179, Proto: "tcp", Len: 100}}
	if ev.Node != "r1" || ev.Iface != "e1-1" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.Pkt.Proto != "tcp" {
		t.Fatalf("expected tcp, got %s", ev.Pkt.Proto)
	}
}

func TestCapabilityResult(t *testing.T) {
	r := CapabilityResult{Available: true, Reason: ""}
	if !r.Available {
		t.Fatal("expected available")
	}
	r = CapabilityResult{Available: false, Reason: "need root"}
	if r.Available || r.Reason != "need root" {
		t.Fatalf("unexpected: %+v", r)
	}
}

// PacketCapturer must now return a PacketEvent stream (not CaptureHandle).
type fakeCapturer struct{}

func (f *fakeCapturer) Capture(ctx context.Context, labName, nodeName, iface string, opts ...CaptureOption) (<-chan PacketEvent, func() error, error) {
	ch := make(chan PacketEvent)
	return ch, func() error { return nil }, nil
}

func TestPacketCapturerInterface(t *testing.T) {
	var _ PacketCapturer = (*fakeCapturer)(nil)
}
