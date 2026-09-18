package capability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCapEff(t *testing.T) {
	// CAP_NET_ADMIN(12) + CAP_SYS_ADMIN(21) + CAP_BPF(39)
	eff, err := parseCapEff("0000008000201000")
	if err != nil {
		t.Fatal(err)
	}
	if !eff.has(12) || !eff.has(21) || !eff.has(39) {
		t.Fatalf("expected bits 12,21,39 set: %+v", eff)
	}
	if eff.has(19) {
		t.Fatal("bit 19 should not be set")
	}
}

func TestParseCapEffInvalid(t *testing.T) {
	if _, err := parseCapEff("not-hex"); err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestCheckTrace(t *testing.T) {
	ok := Capabilities{CapBPF: true, CapNetAdmin: true, CapSysAdmin: true, CapSysPtrace: true, BTF: true}
	if allowed, reason := ok.Check(ReqTrace); !allowed || reason != "" {
		t.Fatalf("expected allowed, got %v %q", allowed, reason)
	}
	miss := Capabilities{CapBPF: true, CapNetAdmin: true, CapSysAdmin: true, BTF: true}
	if allowed, reason := miss.Check(ReqTrace); allowed || reason == "" {
		t.Fatalf("expected denied with reason, got %v %q", allowed, reason)
	}
	if allowed, _ := (Capabilities{Root: true}).Check(ReqTrace); !allowed {
		t.Fatal("root should satisfy ReqTrace")
	}
	noBTF := Capabilities{CapBPF: true, CapNetAdmin: true, CapSysAdmin: true, CapSysPtrace: true}
	if allowed, _ := noBTF.Check(ReqTrace); allowed {
		t.Fatal("missing BTF should deny ReqTrace")
	}
}

func TestCheckDockerAndClab(t *testing.T) {
	if allowed, _ := (Capabilities{DockerGroup: true}).Check(ReqDocker); !allowed {
		t.Fatal("docker group should satisfy ReqDocker")
	}
	if allowed, reason := (Capabilities{}).Check(ReqDocker); allowed || reason == "" {
		t.Fatal("no docker => denied with reason")
	}
	if allowed, _ := (Capabilities{ClabSUID: true}).Check(ReqClabPrivileged); !allowed {
		t.Fatal("SUID should satisfy ReqClabPrivileged")
	}
	if allowed, reason := (Capabilities{}).Check(ReqClabPrivileged); allowed || reason == "" {
		t.Fatal("no SUID/root => denied with reason")
	}
	if allowed, _ := (Capabilities{}).Check(ReqNone); !allowed {
		t.Fatal("ReqNone should always be allowed")
	}
}

func TestDetectSUID(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "clab")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bin, 0o755|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if !Detect(bin).ClabSUID {
		t.Fatal("expected ClabSUID true for setuid file")
	}
	if Detect(filepath.Join(dir, "missing")).ClabSUID {
		t.Fatal("missing binary must not report SUID")
	}
}
