//go:build linux && netns

package linux

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/pktc/internal/nettest"
)

// The chained script is run verbatim against a real kernel. Ordering is
// the thing under test as much as syntax: these commands do not commute,
// and a naive emitter produces a script that fails halfway through.
func TestChainAgainstKernel(t *testing.T) {
	ns := nettest.New(t, "chain", "192.168.1.10")
	ns.RequireType("vxlan")

	if err := ns.RunScript(strings.ReplaceAll(script(t, nested), "dev eth0", "dev veth0")); err != nil {
		t.Fatalf("chained script failed against the kernel:\n%v", err)
	}

	outer, err := ns.Attrs("vxlan100", "vxlan")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outer, "dev veth0") {
		t.Errorf("outer device is not on the underlay:\n  %s", outer)
	}

	inner, err := ns.Attrs("vxlan200", "vxlan")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inner, "dev vxlan100") {
		t.Errorf("inner device is not stacked on the outer one:\n  %s", inner)
	}
	if !strings.Contains(inner, "local 10.100.0.1") {
		t.Errorf("inner tunnel did not take the middle address as its endpoint:\n  %s", inner)
	}

	for _, want := range []struct{ dev, addr string }{
		{"vxlan100", "10.100.0.1/24"},
		{"vxlan200", "10.200.0.1/24"},
	} {
		out, err := ns.Run("ip", "-brief", "addr", "show", want.dev)
		if err != nil {
			t.Fatalf("%s: %v\n%s", want.dev, err, out)
		}
		if !strings.Contains(out, want.addr) {
			t.Errorf("%s is missing %s:\n%s", want.dev, want.addr, out)
		}
		if !strings.Contains(out, "UNKNOWN") && !strings.Contains(out, "UP") {
			t.Errorf("%s was never brought up:\n%s", want.dev, out)
		}
	}
}
