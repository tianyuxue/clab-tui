package components

import (
	"testing"
	"time"
)

func TestToastInitHidden(t *testing.T) {
	to := NewToast()
	if to.Visible() {
		t.Fatal("expected hidden initially")
	}
}

func TestToastShow(t *testing.T) {
	to := NewToast()
	to.Show("hello")
	if !to.Visible() {
		t.Fatal("expected visible after Show")
	}
	if to.Message() != "hello" {
		t.Fatalf("expected message hello, got %q", to.Message())
	}
}

func TestToastHide(t *testing.T) {
	to := NewToast()
	to.Show("msg")
	to.Hide()
	if to.Visible() {
		t.Fatal("expected hidden after Hide")
	}
}

func TestToastShowReturnsTimerCmd(t *testing.T) {
	to := NewToast()
	cmd := to.Show("msg")
	if cmd == nil {
		t.Fatal("expected a timer command from Show")
	}
	// Running the cmd after the duration yields toastExpiredMsg.
	to.ExpireAfter = 1 * time.Millisecond
	cmd = to.Show("msg")
	msg := cmd()
	if msg == nil {
		t.Fatal("expected toastExpiredMsg after timeout")
	}
	if _, ok := msg.(toastExpiredMsg); !ok {
		t.Fatalf("expected toastExpiredMsg, got %T", msg)
	}
}

func TestToastShowResetsMessage(t *testing.T) {
	to := NewToast()
	to.Show("first")
	to.Show("second")
	if to.Message() != "second" {
		t.Fatalf("expected second, got %q", to.Message())
	}
}

func TestToastExpiryHides(t *testing.T) {
	to := NewToast()
	to.ExpireAfter = 1 * time.Millisecond
	cmd := to.Show("msg")
	// Simulate tea loop: cmd -> toastExpiredMsg -> Hide.
	msg := cmd()
	to.HandleExpired(msg)
	if to.Visible() {
		t.Fatal("expected hidden after expiry handled")
	}
}
