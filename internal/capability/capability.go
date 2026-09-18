// Package capability detects the privileges available to the current process so
// callers can degrade gracefully and prompt before privileged operations.
package capability

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

// Linux capability numbers (see linux/capability.h).
const (
	capNetAdmin  = 12
	capSysPtrace = 19
	capSysAdmin  = 21
	capBPF       = 39
)

// Capabilities is a snapshot of the privileges available to the process.
type Capabilities struct {
	Root         bool
	DockerGroup  bool
	ClabSUID     bool
	CapBPF       bool
	CapNetAdmin  bool
	CapSysAdmin  bool
	CapSysPtrace bool
	BTF          bool
}

// Requirement describes the privilege a feature needs.
type Requirement int

const (
	ReqNone           Requirement = iota // always available
	ReqDocker                            // root or docker group
	ReqClabPrivileged                    // root or containerlab installed with SUID
	ReqTrace                             // root or the eBPF capability set + BTF
)

// Check reports whether req is satisfied and, if not, an English reason suitable
// for a user-facing toast.
func (c Capabilities) Check(req Requirement) (bool, string) {
	switch req {
	case ReqNone:
		return true, ""
	case ReqDocker:
		if c.Root || c.DockerGroup {
			return true, ""
		}
		return false, "docker access required: add your user to the docker group or run with sudo"
	case ReqClabPrivileged:
		if c.Root || c.ClabSUID {
			return true, ""
		}
		return false, "containerlab needs root: install it with SUID (sudo chmod u+s $(which containerlab)) or run with sudo"
	case ReqTrace:
		if c.Root {
			return true, ""
		}
		if c.CapBPF && c.CapNetAdmin && c.CapSysAdmin && c.CapSysPtrace && c.BTF {
			return true, ""
		}
		return false, "packet tracing needs root (run with sudo) or CAP_BPF+CAP_NET_ADMIN+CAP_SYS_ADMIN+CAP_SYS_PTRACE"
	}
	return false, "unknown capability requirement"
}

type capEff struct{ bits uint64 }

func (e capEff) has(n int) bool { return e.bits&(1<<uint(n)) != 0 }

func parseCapEff(hexStr string) (capEff, error) {
	v, err := strconv.ParseUint(strings.TrimSpace(hexStr), 16, 64)
	if err != nil {
		return capEff{}, fmt.Errorf("parse CapEff %q: %w", hexStr, err)
	}
	return capEff{bits: v}, nil
}

// Detect inspects the current process/environment. clabBinPath is the
// containerlab binary path (used for the SUID check); empty is allowed.
func Detect(clabBinPath string) Capabilities {
	c := Capabilities{Root: os.Geteuid() == 0}
	if eff, ok := readCapEff(); ok {
		c.CapBPF = eff.has(capBPF)
		c.CapNetAdmin = eff.has(capNetAdmin)
		c.CapSysAdmin = eff.has(capSysAdmin)
		c.CapSysPtrace = eff.has(capSysPtrace)
	}
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
		c.BTF = true
	}
	if clabBinPath != "" {
		if fi, err := os.Stat(clabBinPath); err == nil && fi.Mode()&os.ModeSetuid != 0 {
			c.ClabSUID = true
		}
	}
	c.DockerGroup = inDockerGroup()
	return c
}

func readCapEff() (capEff, bool) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return capEff{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "CapEff:") {
			if e, err := parseCapEff(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))); err == nil {
				return e, true
			}
			return capEff{}, false
		}
	}
	return capEff{}, false
}

func inDockerGroup() bool {
	gids, err := os.Getgroups()
	if err != nil {
		return false
	}
	g, err := user.LookupGroup("docker")
	if err != nil {
		return false
	}
	dockerGID, err := strconv.Atoi(g.Gid)
	if err != nil {
		return false
	}
	for _, id := range gids {
		if id == dockerGID {
			return true
		}
	}
	return false
}
