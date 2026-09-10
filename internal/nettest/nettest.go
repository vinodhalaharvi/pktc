//go:build linux

// Package nettest runs generated configuration against a real kernel.
//
// Golden tests prove pktc emits the commands we intended. They cannot
// prove those commands are valid, that the flag spelling is current, or
// that the resulting device has the attributes we claimed. Only the
// kernel can answer that, so these tests create a network namespace,
// run the script, and read the device back.
//
// Everything here is guarded: the tests skip rather than fail when the
// environment cannot support them.
package nettest

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Require skips the test unless this machine can create namespaces.
func Require(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("netns tests need root (CAP_NET_ADMIN)")
	}
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("netns tests need iproute2")
	}
	if out, err := exec.Command("ip", "netns", "list").CombinedOutput(); err != nil {
		t.Skipf("namespaces unavailable: %v: %s", err, out)
	}
}

// NS is a network namespace that is deleted when the test ends.
type NS struct {
	Name string
	t    *testing.T
}

// New creates a namespace with a veth pair standing in for the
// underlay, addressed as local.
func New(t *testing.T, name, local string) *NS {
	t.Helper()
	Require(t)

	full := fmt.Sprintf("pktc-%s-%d", name, os.Getpid())
	if out, err := exec.Command("ip", "netns", "add", full).CombinedOutput(); err != nil {
		t.Skipf("cannot create namespace: %v: %s", err, out)
	}
	ns := &NS{Name: full, t: t}
	t.Cleanup(func() { _ = exec.Command("ip", "netns", "del", full).Run() })

	ns.MustRun("ip", "link", "add", "veth0", "type", "veth", "peer", "name", "veth1")
	ns.MustRun("ip", "addr", "add", local+"/24", "dev", "veth0")
	ns.MustRun("ip", "link", "set", "veth0", "up")
	return ns
}

// Run executes one command inside the namespace.
func (n *NS) Run(argv ...string) (string, error) {
	args := append([]string{"netns", "exec", n.Name}, argv...)
	out, err := exec.Command("ip", args...).CombinedOutput()
	return string(out), err
}

// MustRun fails the test if the command does not succeed.
func (n *NS) MustRun(argv ...string) string {
	n.t.Helper()
	out, err := n.Run(argv...)
	if err != nil {
		n.t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
	}
	return out
}

// RunScript executes a generated script line by line, so a failure
// names the command that failed rather than the whole script.
func (n *NS) RunScript(script string) error {
	for _, line := range strings.Split(strings.TrimSpace(script), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if out, err := n.Run(strings.Fields(line)...); err != nil {
			return fmt.Errorf("%s: %v\n%s", line, err, strings.TrimSpace(out))
		}
	}
	return nil
}

// Show returns the kernel's own description of a device.
func (n *NS) Show(dev string) (string, error) {
	return n.Run("ip", "-details", "link", "show", dev)
}

// SupportsType skips the test when the kernel cannot create a device of
// this kind. Containers routinely lack tunnel modules, and that is a
// property of the machine rather than a bug in pktc.
func (n *NS) SupportsType(kind string) bool {
	probe := "pktc-probe0"
	out, err := n.Run("ip", "link", "add", "name", probe, "type", kind)
	if err != nil {
		return !strings.Contains(out, "Unknown device type") &&
			!strings.Contains(out, "not supported")
	}
	_, _ = n.Run("ip", "link", "del", probe)
	return true
}

// RequireType skips unless the device kind is available.
func (n *NS) RequireType(kind string) {
	n.t.Helper()
	if !n.SupportsType(kind) {
		n.t.Skipf("kernel has no %s device support here", kind)
	}
}
