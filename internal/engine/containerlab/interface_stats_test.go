package containerlab

import (
	"testing"
	"time"
)

func TestParseIPLinkStatsJSON(t *testing.T) {
	out := `[{
  "ifname":"e1-1",
  "stats64":{
    "rx":{"bytes":1000,"packets":10,"errors":1,"dropped":2},
    "tx":{"bytes":2000,"packets":20,"errors":3,"dropped":4}
  }
}]`
	got, err := parseIPLinkStatsJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	stats := got["e1-1"]
	if stats.RxBytes != 1000 || stats.RxPackets != 10 || stats.TxBytes != 2000 || stats.TxPackets != 20 {
		t.Fatalf("unexpected counters: %+v", stats)
	}
}

func TestParseProcNetDevStats(t *testing.T) {
	out := "Inter-| Receive | Transmit\n" +
		" face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo collsns carrier compressed\n" +
		"e1-1: 1000 10 1 2 0 0 0 0 2000 20 3 4 0 0 0 0\n"
	got, err := parseProcNetDevStats(out)
	if err != nil {
		t.Fatal(err)
	}
	stats := got["e1-1"]
	if stats.RxBytes != 1000 || stats.RxPackets != 10 || stats.TxBytes != 2000 || stats.TxPackets != 20 {
		t.Fatalf("unexpected counters: %+v", stats)
	}
}

func TestInterfaceStatsRate(t *testing.T) {
	prev := interfaceStatsCounters{RxBytes: 1000, TxBytes: 2000, At: time.Unix(100, 0)}
	next := interfaceStatsCounters{RxBytes: 3000, TxBytes: 5000, At: time.Unix(102, 0)}
	got := interfaceStatsRate(prev, next)
	if got.RxBps != 1000 || got.TxBps != 1500 || got.Interval != 2*time.Second {
		t.Fatalf("unexpected rates: %+v", got)
	}
}

func TestParseIPLinkStatsJSONRejectsInvalidInput(t *testing.T) {
	if _, err := parseIPLinkStatsJSON("invalid"); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
