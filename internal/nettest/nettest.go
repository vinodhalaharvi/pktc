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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
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
//
// Each line goes through a shell, because the emitted script is a shell
// script: quoting and command substitution are part of what is being
// tested. Splitting on whitespace and exec'ing directly would hand
// "$(cat key.pub)" to wg as a literal string, which fails in a way that
// looks like a broken tile rather than a broken harness.
func (n *NS) RunScript(script string) error {
	for _, line := range strings.Split(strings.TrimSpace(script), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if out, err := n.Run("sh", "-c", line); err != nil {
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

// Attrs returns just the driver attribute line for a device: the line
// beginning "vxlan ...", "gre ...", and so on.
//
// Assertions belong here rather than against the whole of
// "ip -details link show", where unrelated netdev fields collide with
// tunnel ones. Every device reports "group default" for its link group,
// which has nothing to do with a VXLAN multicast group.
func (n *NS) Attrs(dev, kind string) (string, error) {
	out, err := n.Show(dev)
	if err != nil {
		return "", fmt.Errorf("%v\n%s", err, out)
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, kind+" ") {
			return line, nil
		}
	}
	return "", fmt.Errorf("no %s attribute line for %s in:\n%s", kind, dev, out)
}

// Pair is two namespaces joined by a veth pair: two machines on one
// underlay, which is the smallest arrangement in which a tunnel is a
// tunnel rather than a device talking to itself.
//
// The underlay is called wire0 inside both namespaces, so the same
// -underlay name works for either end.
type Pair struct {
	A, B         *NS
	AAddr, BAddr string
}

// NewPair builds the two namespaces and addresses their underlays.
func NewPair(t *testing.T, name, aAddr, bAddr string) *Pair {
	t.Helper()
	Require(t)

	a := newEmpty(t, name+"a")
	b := newEmpty(t, name+"b")

	ta := "pktc-ta-" + fmt.Sprint(os.Getpid())
	tb := "pktc-tb-" + fmt.Sprint(os.Getpid())
	if out, err := exec.Command("ip", "link", "add", ta, "type", "veth", "peer", "name", tb).CombinedOutput(); err != nil {
		t.Skipf("cannot create a veth pair: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("ip", "link", "del", ta).Run() })

	mustHost(t, "ip", "link", "set", ta, "netns", a.Name)
	mustHost(t, "ip", "link", "set", tb, "netns", b.Name)

	for _, e := range []struct {
		ns   *NS
		from string
		addr string
	}{{a, ta, aAddr}, {b, tb, bAddr}} {
		e.ns.MustRun("ip", "link", "set", e.from, "name", "wire0")
		e.ns.MustRun("ip", "addr", "add", e.addr+"/24", "dev", "wire0")
		e.ns.MustRun("ip", "link", "set", "wire0", "up")
		e.ns.MustRun("ip", "link", "set", "lo", "up")
	}
	return &Pair{A: a, B: b, AAddr: aAddr, BAddr: bAddr}
}

func newEmpty(t *testing.T, name string) *NS {
	t.Helper()
	full := fmt.Sprintf("pktc-%s-%d", name, os.Getpid())
	if out, err := exec.Command("ip", "netns", "add", full).CombinedOutput(); err != nil {
		t.Skipf("cannot create namespace: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("ip", "netns", "del", full).Run() })
	return &NS{Name: full, t: t}
}

func mustHost(t *testing.T, argv ...string) {
	t.Helper()
	if out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
	}
}

// Capture is a running tcpdump.
type Capture struct {
	cmd *exec.Cmd
	buf *bytes.Buffer
	t   *testing.T
}

// StartCapture begins capturing on a device and returns once tcpdump is
// listening, so traffic generated afterwards is not missed.
func (n *NS) StartCapture(dev, filter string, count int) *Capture {
	n.t.Helper()
	if _, err := exec.LookPath("tcpdump"); err != nil {
		n.t.Skip("wire verification needs tcpdump")
	}
	args := []string{"netns", "exec", n.Name, "tcpdump",
		"-i", dev, "-c", fmt.Sprint(count), "-nn", "-l", "-U", filter}
	cmd := exec.Command("ip", args...)
	buf := &bytes.Buffer{}
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := cmd.Start(); err != nil {
		n.t.Skipf("cannot start tcpdump: %v", err)
	}
	time.Sleep(700 * time.Millisecond) // let the socket attach
	return &Capture{cmd: cmd, buf: buf, t: n.t}
}

// Wait stops the capture and returns what it saw.
func (c *Capture) Wait(d time.Duration) string {
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		_ = c.cmd.Process.Kill()
		<-done
	}
	return c.buf.String()
}

// SendUDP pushes datagrams at an address, using the shell's /dev/udp so
// the test needs no helper binary inside the namespace.
func (n *NS) SendUDP(addr string, port, times int) error {
	if _, err := exec.LookPath("bash"); err != nil {
		n.t.Skip("traffic generation needs bash")
	}
	script := fmt.Sprintf(
		"for i in $(seq %d); do echo pktc > /dev/udp/%s/%d 2>/dev/null || true; sleep 0.2; done",
		times, addr, port)
	out, err := n.Run("bash", "-c", script)
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}
