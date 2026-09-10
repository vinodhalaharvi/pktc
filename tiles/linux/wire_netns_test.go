//go:build linux && netns

package linux

import (
	"strings"
	"testing"
	"time"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/internal/nettest"
	"github.com/vinodhalaharvi/pktc/mirror"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
	"github.com/vinodhalaharvi/pktc/wire"
)

func parse(t *testing.T, src string) *ast.Node {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		t.Fatal(err)
	}
	root, err := ast.Build(forms[0])
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func lowerTree(t *testing.T, root *ast.Node, underlay string) string {
	t.Helper()
	sp, err := spine.FromForm(root)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	res, err := reg.Cover(sp, env.New(underlay))
	if err != nil {
		t.Fatal(err)
	}
	return res.Script.Plain()
}

// TestWireMatchesTheTree is the loop nothing else closes.
//
// One description configures the near end, is turned around to
// configure the far end, and predicts what the underlay should carry.
// Traffic is then pushed through the tunnel and the underlay captured.
// If the capture and the prediction disagree, one of those readings of
// the tree is wrong -- and no amount of golden output would have said so.
func TestWireMatchesTheTree(t *testing.T) {
	const src = `(configure
	  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
	    (udp :dst-port 4789
	      (vxlan :vni 100
	        (ethernet
	          (ipv4 :src "10.100.0.1/24"))))))`

	p := nettest.NewPair(t, "wire", "192.168.1.10", "192.168.1.20")
	p.A.RequireType("vxlan")

	near := parse(t, src)
	if err := p.A.RunScript(lowerTree(t, near, "wire0")); err != nil {
		t.Fatalf("near end failed:\n%v", err)
	}

	far, err := mirror.Peer(parse(t, src), "10.100.0.2/24")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.B.RunScript(lowerTree(t, far.Tree, "wire0")); err != nil {
		t.Fatalf("far end failed:\n%v", err)
	}

	// The prediction comes from the same tree that produced both scripts.
	sp, err := spine.FromForm(near)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := wire.Expect(sp)
	if err != nil {
		t.Fatal(err)
	}

	cap := p.B.StartCapture("wire0", exp.Filter, 4)
	if err := p.A.SendUDP("10.100.0.2", 9999, 4); err != nil {
		t.Fatalf("could not generate traffic: %v", err)
	}
	captured := cap.Wait(8 * time.Second)

	if strings.TrimSpace(captured) == "" {
		t.Fatalf("nothing matched %q on the underlay; the tunnel carried no traffic", exp.Filter)
	}
	for _, missing := range exp.Check(captured) {
		t.Errorf("the tree says %s is %s, but the wire does not show it (looking for %q)\n--- captured ---\n%s",
			missing.Label, missing.Value, missing.Match, captured)
	}
	if !t.Failed() {
		t.Logf("wire agrees with the tree:\n%s", firstLines(captured, 2))
	}
}

// firstLines drops tcpdump's preamble and returns the packet lines.
func firstLines(s string, n int) string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.HasPrefix(l, "tcpdump:") || strings.HasPrefix(l, "listening on") ||
			strings.Contains(l, "packets captured") || strings.Contains(l, "packets received") ||
			strings.Contains(l, "packets dropped") || strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(l))
		if len(out) == n {
			break
		}
	}
	return strings.Join(out, "\n")
}

// A deliberately wrong prediction must fail, or the loop proves nothing.
func TestWireCheckDetectsDisagreement(t *testing.T) {
	exp := wire.Expectation{Facts: []wire.Fact{
		{Label: "VXLAN VNI", Value: "999", Match: "vni 999"},
	}}
	captured := "192.168.1.10.34533 > 192.168.1.20.4789: VXLAN, flags [I] (0x08), vni 100"
	if len(exp.Check(captured)) != 1 {
		t.Error("Check must notice a VNI the wire does not carry")
	}
}
