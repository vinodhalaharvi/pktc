//go:build linux && netns

package linux

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/pktc/internal/nettest"
)

// Each VXLAN mode is run against a real kernel and the device is read
// back. The golden tests prove we emitted what we meant; only the
// kernel can say whether the device came out the way we claimed.
func TestVXLANAgainstKernel(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		dev    string
		expect []string
		reject []string
	}{
		{
			name: "point to point",
			src: `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
				(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`,
			dev:    "vxlan100",
			expect: []string{"id 100", "remote 192.168.1.20", "local 192.168.1.10", "dstport 4789"},
			reject: []string{"dstport 8472"},
		},
		{
			name: "multicast group",
			src: `(configure (ipv4 :src "192.168.1.10" :dst "239.1.1.1"
				(udp :dst-port 4789 (vxlan :vni 101 (ethernet)))))`,
			dev:    "vxlan101",
			expect: []string{"id 101", "group 239.1.1.1", "dstport 4789"},
			reject: []string{"remote 239.1.1.1"},
		},
		{
			name: "learned peers",
			src: `(configure (ipv4 :src "192.168.1.10" :dst *
				(udp :dst-port 4789 (vxlan :vni 102 (ethernet)))))`,
			dev:    "vxlan102",
			expect: []string{"id 102", "local 192.168.1.10", "dstport 4789"},
			reject: []string{"remote ", "group "},
		},
		{
			name: "every vni",
			src: `(configure (ipv4 :src "192.168.1.10" :dst *
				(udp :dst-port 4789 (vxlan :vni * (ethernet)))))`,
			dev:    "vxlan1",
			expect: []string{"external", "dstport 4789"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := nettest.New(t, "vx", "192.168.1.10")
			ns.RequireType("vxlan")

			if err := ns.RunScript(script(t, tt.src)); err != nil {
				t.Fatalf("generated script failed against the kernel:\n%v", err)
			}
			attrs, err := ns.Attrs(tt.dev, "vxlan")
			if err != nil {
				t.Fatalf("device %s was not created as expected: %v", tt.dev, err)
			}
			for _, want := range tt.expect {
				if !strings.Contains(attrs, want) {
					t.Errorf("device is missing %q\n  %s", want, attrs)
				}
			}
			for _, bad := range tt.reject {
				if strings.Contains(attrs, bad) {
					t.Errorf("device unexpectedly has %q\n  %s", bad, attrs)
				}
			}
		})
	}
}

// The kernel rejects 'external' together with an id. That exclusion is
// already expressed by the '*' states, so pktc can never emit both --
// but if the tile ever regressed, this is the failure it would cause.
func TestExternalAndIDAreExclusive(t *testing.T) {
	ns := nettest.New(t, "vxexcl", "192.168.1.10")
	ns.RequireType("vxlan")

	out, err := ns.Run("ip", "link", "add", "name", "vxbad", "type", "vxlan",
		"external", "id", "100", "dstport", "4789", "dev", "veth0")
	if err == nil {
		t.Fatal("the kernel accepted external together with an id; " +
			"the VXLAN tile's mode exclusion assumes it does not")
	}
	if !strings.Contains(out, "external") {
		t.Logf("rejected, though not for the expected reason: %s", strings.TrimSpace(out))
	}
}
