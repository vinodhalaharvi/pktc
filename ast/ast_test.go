package ast

import (
	"net/netip"
	"testing"

	"github.com/vinodhalaharvi/pktc/sexp"
)

func build(t *testing.T, src string) *Node {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	n, err := Build(forms[0])
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return n
}

func TestBuildShape(t *testing.T) {
	n := build(t, `(ipv4 :src "10.0.0.1" :dst "10.0.0.2" (udp :dst-port 4789))`)
	if n.Name != "ipv4" {
		t.Errorf("name = %q", n.Name)
	}
	if len(n.Props) != 2 {
		t.Errorf("got %d props, want 2", len(n.Props))
	}
	if len(n.Children) != 1 || n.Children[0].Name != "udp" {
		t.Errorf("children = %v", n.Children)
	}
}

func TestBuildErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"empty form", `()`},
		{"keyword head", `(:ipv4)`},
		{"dangling property", `(udp :dst-port)`},
		{"property without value", `(udp :dst-port :src-port 1)`},
		{"duplicate property", `(udp :dst-port 1 :dst-port 2)`},
		{"property after child", `(ipv4 (udp) :src "10.0.0.1")`},
		{"bare atom child", `(ipv4 udp)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forms, err := sexp.Read("t.lisp", tt.src)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if _, err := Build(forms[0]); err == nil {
				t.Fatalf("expected an error for %q", tt.src)
			}
		})
	}
}

func TestThreeStates(t *testing.T) {
	n := build(t, `(vxlan :vni 100)`)
	v, err := Get(n, VNI)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v.Get(); !ok || got != 100 {
		t.Errorf("concrete: got %v ok=%v", got, ok)
	}

	n = build(t, `(vxlan :vni *)`)
	v, err = Get(n, VNI)
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsDynamic() {
		t.Errorf("expected dynamic, got %v", v.State())
	}
	if _, err := v.Require("vxlan"); err == nil {
		t.Error("Require should reject a dynamic value")
	}

	n = build(t, `(vxlan)`)
	v, err = Get(n, VNI)
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsAbsent() {
		t.Errorf("expected absent, got %v", v.State())
	}
	if _, err := v.Require("vxlan"); err == nil {
		t.Error("Require should reject an absent value")
	}
}

func TestVNIRange(t *testing.T) {
	n := build(t, `(vxlan :vni 16777216)`)
	if _, err := Get(n, VNI); err == nil {
		t.Fatal("expected a 24-bit range error")
	}
	n = build(t, `(vxlan :vni 16777215)`)
	if _, err := Get(n, VNI); err != nil {
		t.Fatalf("16777215 should be valid: %v", err)
	}
}

func TestAddrAndPrefix(t *testing.T) {
	n := build(t, `(ipv4 :src "10.0.0.1")`)
	v, err := Get(n, Src)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := v.Get()
	if got != netip.MustParseAddr("10.0.0.1") {
		t.Errorf("got %v", got)
	}

	// A bare address is not a prefix: the length is not in the packet.
	if _, err := Get(n, SrcPrefix); err == nil {
		t.Error("bare address should be rejected as a prefix")
	}

	n = build(t, `(ipv4 :src "10.0.0.1/24")`)
	p, err := Get(n, SrcPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if pp, _ := p.Get(); pp != netip.MustParsePrefix("10.0.0.1/24") {
		t.Errorf("got %v", pp)
	}
}

func TestHexAndPortRange(t *testing.T) {
	n := build(t, `(esp :spi 0x1001)`)
	v, err := Get(n, SPI)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := v.Get(); got != 0x1001 {
		t.Errorf("got %#x", got)
	}
	n = build(t, `(udp :dst-port 70000)`)
	if _, err := Get(n, DstPort); err == nil {
		t.Fatal("expected a 16-bit range error")
	}
}
