package mirror

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/sexp"
)

func tree(t *testing.T, src string) *ast.Node {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	n, err := ast.Build(forms[0])
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return n
}

func prop(t *testing.T, n *ast.Node, key string) string {
	t.Helper()
	p, ok := n.Lookup(key)
	if !ok {
		return ""
	}
	return p.Val.Text
}

// packet returns the outermost protocol node below (configure ...).
func packet(n *ast.Node) *ast.Node { return n.Children[0] }

func TestEndpointsSwap(t *testing.T) {
	res, err := Peer(tree(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(gre (ipv4 :src "10.20.0.1/30"))))`), "10.20.0.2/30")
	if err != nil {
		t.Fatal(err)
	}
	outer := packet(res.Tree)
	if got := prop(t, outer, "src"); got != "192.168.1.20" {
		t.Errorf("peer :src = %q, want the near end's :dst", got)
	}
	if got := prop(t, outer, "dst"); got != "192.168.1.10" {
		t.Errorf("peer :dst = %q, want the near end's :src", got)
	}
}

func TestOriginalIsUntouched(t *testing.T) {
	root := tree(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(gre (ipv4 :src "10.20.0.1/30"))))`)
	if _, err := Peer(root, "10.20.0.2/30"); err != nil {
		t.Fatal(err)
	}
	if got := prop(t, packet(root), "src"); got != "192.168.1.10" {
		t.Errorf("the near end's tree was mutated: :src = %q", got)
	}
}

// A VNI, a GRE key and an ESP SPI identify the tunnel rather than an
// end of it, so they must survive the mirror unchanged.
func TestSymmetricFieldsSurvive(t *testing.T) {
	res, err := Peer(tree(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`), "")
	if err != nil {
		t.Fatal(err)
	}
	vxlan := packet(res.Tree).Children[0].Children[0]
	if got := prop(t, vxlan, "vni"); got != "100" {
		t.Errorf("peer :vni = %q, want 100 unchanged", got)
	}
}

// :src X :dst * mirrors to :src * :dst X. The end that learned its
// peers becomes the end that is learned, which is the correct reading
// rather than an accident of the implementation.
func TestStarSwapsWithTheValue(t *testing.T) {
	res, err := Peer(tree(t, `(configure (ipv4 :src "192.168.1.10" :dst *
		(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`), "")
	if err != nil {
		t.Fatal(err)
	}
	outer := packet(res.Tree)
	if got := prop(t, outer, "src"); got != "*" {
		t.Errorf("peer :src = %q, want *", got)
	}
	if got := prop(t, outer, "dst"); got != "192.168.1.10" {
		t.Errorf("peer :dst = %q, want the near end's :src", got)
	}
}

func TestInnerAddressIsReportedMissing(t *testing.T) {
	res, err := Peer(tree(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(gre (ipv4 :src "10.20.0.1/30"))))`), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Missing) != 1 || !strings.Contains(res.Missing[0], "host part") {
		t.Fatalf("Missing = %v, want one note about the host part", res.Missing)
	}
	inner := packet(res.Tree).Children[0].Children[0] // ipv4 > gre > ipv4
	if _, ok := inner.Lookup("src"); ok {
		t.Error("an undetermined inner address must be dropped, not guessed")
	}
}

func TestInnerAddressSubstituted(t *testing.T) {
	res, err := Peer(tree(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(gre (ipv4 :src "10.20.0.1/30"))))`), "10.20.0.2/30")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Missing) != 0 {
		t.Errorf("Missing = %v, want none", res.Missing)
	}
	inner := packet(res.Tree).Children[0].Children[0] // ipv4 > gre > ipv4
	if got := prop(t, inner, "src"); got != "10.20.0.2/30" {
		t.Errorf("inner :src = %q", got)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Peer(tree(t, `(configure (ipv4 :src "192.168.1.10" (gre (ipv4 :src "10.0.0.1/30"))))`), ""); err == nil {
		t.Error("mirroring without :dst must fail")
	}
	if _, err := Peer(tree(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre (ipv4 :src "10.0.0.1/30"))))`), "10.0.0.2"); err == nil {
		t.Error("a peer address without a prefix length must be rejected")
	}
}

// A nested stack has more than one two-ended layer. All of them turn
// around, not just the outermost.
func TestNestedLayersAllSwap(t *testing.T) {
	res, err := Peer(tree(t, `(configure
		(ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		  (udp :dst-port 4789
		    (vxlan :vni 100
		      (ethernet
		        (ipv4 :src "10.100.0.1/24" :dst "10.100.0.2"
		          (udp :dst-port 4789
		            (vxlan :vni 200
		              (ethernet
		                (ipv4 :src "10.200.0.1/24")))))))))) `), "10.200.0.2/24")
	if err != nil {
		t.Fatal(err)
	}
	outer := packet(res.Tree)
	if got := prop(t, outer, "src"); got != "192.168.1.20" {
		t.Errorf("outer :src = %q", got)
	}
	middle := outer.Children[0].Children[0].Children[0] // ipv4 > udp > vxlan > ethernet
	middle = middle.Children[0]                         // > ipv4
	if got := prop(t, middle, "src"); got != "10.100.0.2/24" {
		t.Errorf("middle :src = %q, want the peer's host part with the length kept", got)
	}
	if got := prop(t, middle, "dst"); got != "10.100.0.1" {
		t.Errorf("middle :dst = %q, want the near end's address without a length", got)
	}
}

// The prefix length says how wide the shared subnet is and belongs to
// whichever field is local. It must not travel with the address.
func TestPrefixLengthStaysWithTheLocalField(t *testing.T) {
	res, err := Peer(tree(t, `(configure (ipv4 :src "10.0.0.1/24" :dst "10.0.0.2"
		(gre (ipv4 :src "172.16.0.1/30"))))`), "172.16.0.2/30")
	if err != nil {
		t.Fatal(err)
	}
	outer := packet(res.Tree)
	if got := prop(t, outer, "src"); got != "10.0.0.2/24" {
		t.Errorf("peer :src = %q, want 10.0.0.2/24", got)
	}
	if got := prop(t, outer, "dst"); got != "10.0.0.1" {
		t.Errorf("peer :dst = %q, want a bare address", got)
	}
}

// The first asymmetry that is structural rather than an address. No
// reading of one end's tree produces the other end's key material, and
// saying so beats emitting a script that looks complete.
func TestWireGuardKeysAreNotDerivable(t *testing.T) {
	res, err := Peer(tree(t, `(configure (ipv4 :src "198.51.100.10" :dst "198.51.100.20"
		(udp :src-port 51820 :dst-port 51820 (wireguard :peer "peer-b" (ipv4 :src "10.44.0.1/24")))))`),
		"10.44.0.2/24")
	if err != nil {
		t.Fatal(err)
	}
	var keys, peer bool
	for _, m := range res.Missing {
		if strings.Contains(m, "private key") {
			keys = true
		}
		if strings.Contains(m, "peer-b") {
			peer = true
		}
	}
	if !keys {
		t.Errorf("the mirror must report key material as underivable, got %v", res.Missing)
	}
	if !peer {
		t.Errorf("the mirror must note that the peer name does not turn around, got %v", res.Missing)
	}
	// The parts it can do, it still does.
	if got := prop(t, packet(res.Tree), "src"); got != "198.51.100.20" {
		t.Errorf("endpoints should still swap, got :src = %q", got)
	}
}
