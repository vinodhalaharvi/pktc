//go:build linux && netns

// These tests run the generated configuration against a real kernel.
//
//	go test -tags netns ./...     (needs root; skips otherwise)
//
// They exist to catch the failure golden tests cannot see: a script
// that is exactly what we meant to emit and still wrong, because a flag
// was renamed, a default is not what we assumed, or the device comes up
// with attributes we did not intend.
package linux

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/internal/nettest"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
)

func script(t *testing.T, src string) string {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		t.Fatal(err)
	}
	root, err := ast.Build(forms[0])
	if err != nil {
		t.Fatal(err)
	}
	sp, err := spine.FromForm(root)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	res, err := reg.Cover(sp, env.New("veth0"))
	if err != nil {
		t.Fatal(err)
	}
	return res.Script.Plain()
}

func TestAgainstKernel(t *testing.T) {
	tests := []struct {
		name   string
		kind   string // kernel device type this needs
		src    string
		dev    string
		expect []string // substrings of the kernel's own description
	}{
		{
			name: "gre",
			kind: "gre",
			src:  `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre (ipv4 :src "10.20.0.1/30"))))`,
			dev:  "gre1",
			expect: []string{
				"remote 192.168.1.20", "local 192.168.1.10",
			},
		},
		{
			name: "gretap",
			kind: "gretap",
			src:  `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre :key 500 (ethernet (ipv4 :src "10.50.0.1/24")))))`,
			dev:  "gretap1",
			expect: []string{
				"remote 192.168.1.20", "local 192.168.1.10",
				"key 500",
			},
		},
		{
			name: "ipip",
			kind: "ipip",
			src:  `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (ipv4 :src "10.30.0.1/30" (tcp :dst-port 22))))`,
			dev:  "ipip1",
			expect: []string{
				"remote 192.168.1.20", "local 192.168.1.10",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := nettest.New(t, tt.name, "192.168.1.10")
			ns.RequireType(tt.kind)

			if err := ns.RunScript(script(t, tt.src)); err != nil {
				t.Fatalf("generated script failed against the kernel:\n%v", err)
			}
			attrs, err := ns.Attrs(tt.dev, tt.kind)
			if err != nil {
				t.Fatalf("device %s was not created as expected: %v", tt.dev, err)
			}
			for _, want := range tt.expect {
				if !strings.Contains(attrs, want) {
					t.Errorf("device is missing %q\n  %s", want, attrs)
				}
			}
			out, err := ns.Show(tt.dev)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "UP") {
				t.Errorf("%s was created but is not up:\n%s", tt.dev, out)
			}
		})
	}
}

// TestVXLANPortDefault records a fact the compiler depends on: the
// kernel's VXLAN port is 8472, not the IANA-assigned 4789. A tile that
// omitted dstport would produce a device that comes up cleanly and
// carries no traffic, which is why omission is an error rather than a
// default.
func TestVXLANPortDefault(t *testing.T) {
	ns := nettest.New(t, "vxdefault", "192.168.1.10")
	ns.RequireType("vxlan")

	ns.MustRun("ip", "link", "add", "name", "vxdef", "type", "vxlan",
		"id", "100", "local", "192.168.1.10", "remote", "192.168.1.20", "dev", "veth0")
	out, err := ns.Show("vxdef")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dstport 8472") {
		t.Errorf("expected the non-standard default 8472; if this changed, "+
			"the VXLAN tile's reasoning needs revisiting:\n%s", out)
	}

	ns.MustRun("ip", "link", "add", "name", "vxiana", "type", "vxlan",
		"id", "101", "dstport", "4789", "local", "192.168.1.10", "dev", "veth0")
	out, err = ns.Show("vxiana")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dstport 4789") {
		t.Errorf("explicit dstport was not honoured:\n%s", out)
	}
}

// TestVXLANDynamicModes checks the two syntaxes the star lowers to,
// ahead of the VXLAN tile itself.
func TestVXLANDynamicModes(t *testing.T) {
	ns := nettest.New(t, "vxdyn", "192.168.1.10")
	ns.RequireType("vxlan")

	// :dst * — many remotes, so a multicast group rather than a remote.
	ns.MustRun("ip", "link", "add", "name", "vxgrp", "type", "vxlan",
		"id", "200", "group", "239.1.1.1", "dstport", "4789", "dev", "veth0")
	if out, _ := ns.Show("vxgrp"); !strings.Contains(out, "group 239.1.1.1") {
		t.Errorf("group mode not applied:\n%s", out)
	}

	// :vni * — one device for every VNI, driven by route metadata.
	ns.MustRun("ip", "link", "add", "name", "vxext", "type", "vxlan",
		"external", "dstport", "4789", "dev", "veth0")
	if out, _ := ns.Show("vxext"); !strings.Contains(out, "external") {
		t.Errorf("external (collect-metadata) mode not applied:\n%s", out)
	}
}

// VLAN and WireGuard were written where their kernel modules were
// unavailable, so these are their first real check. A skip here means
// the module is missing, not that the tile is right.
func TestNewTilesAgainstKernel(t *testing.T) {
	tests := []struct {
		name   string
		kind   string
		src    string
		dev    string
		expect []string
	}{
		{
			name:   "vlan",
			kind:   "vlan",
			src:    `(configure (ethernet (vlan :id 100 (ipv4 :src "10.0.0.1/24"))))`,
			dev:    "veth0.100",
			expect: []string{"id 100"},
		},
		{
			name: "wireguard",
			kind: "wireguard",
			src: `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
				(udp :src-port 51820 :dst-port 51820 (wireguard :peer "peer-b" (ipv4 :src "10.44.0.1/24")))))`,
			dev:    "wg1",
			expect: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := nettest.New(t, tt.name, "192.168.1.10")
			ns.RequireType(tt.kind)

			s := strings.ReplaceAll(script(t, tt.src), "dev eth0", "dev veth0")
			s = strings.ReplaceAll(s, "link eth0 name eth0.", "link veth0 name veth0.")
			if tt.name == "wireguard" {
				// Key material is machine state; make some so the
				// generated script has something real to read.
				ns.MustRun("sh", "-c",
					"mkdir -p /etc/wireguard && "+
						"(wg genkey > /etc/wireguard/wg1.key 2>/dev/null || echo skip) && "+
						"(wg genkey | wg pubkey > /etc/wireguard/peer-b.pub 2>/dev/null || echo skip)")
				if _, err := ns.Run("sh", "-c", "test -s /etc/wireguard/wg1.key"); err != nil {
					t.Skip("wireguard tools are not installed")
				}
			}
			if err := ns.RunScript(s); err != nil {
				t.Fatalf("generated script failed against the kernel:\n%v", err)
			}
			if len(tt.expect) == 0 {
				if out, err := ns.Show(tt.dev); err != nil {
					t.Fatalf("device %s was not created: %v\n%s", tt.dev, err, out)
				}
				return
			}
			attrs, err := ns.Attrs(tt.dev, tt.kind)
			if err != nil {
				t.Fatalf("device %s was not created as expected: %v", tt.dev, err)
			}
			for _, want := range tt.expect {
				if !strings.Contains(attrs, want) {
					t.Errorf("device is missing %q\n  %s", want, attrs)
				}
			}
		})
	}
}
