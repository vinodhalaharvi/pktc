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

// TestSemanticsAgainstKernel is the loop nothing else closes, run over
// every tile that can be stood up end to end.
//
// One description configures the near end, is turned around to
// configure the far end, and predicts what the underlay should carry.
// Traffic is pushed through the tunnel and the underlay captured. If
// the capture and the prediction disagree, one of those readings of the
// tree is wrong — and no amount of golden output would have said so.
func TestSemanticsAgainstKernel(t *testing.T) {
	tests := []struct {
		name      string
		kind      string // kernel device type this needs
		src       string
		peerInner string // the far end's inner address
		target    string // where to send, inside the tunnel
	}{
		{
			name: "vxlan",
			kind: "vxlan",
			src: `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
				(udp :dst-port 4789 (vxlan :vni 100 (ethernet (ipv4 :src "10.100.0.1/24"))))))`,
			peerInner: "10.100.0.2/24",
			target:    "10.100.0.2",
		},
		{
			name:      "gre",
			kind:      "gre",
			src:       `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre (ipv4 :src "10.20.0.1/30"))))`,
			peerInner: "10.20.0.2/30",
			target:    "10.20.0.2",
		},
		{
			name: "gretap",
			kind: "gretap",
			src: `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
				(gre (ethernet (ipv4 :src "10.50.0.1/24")))))`,
			peerInner: "10.50.0.2/24",
			target:    "10.50.0.2",
		},
		{
			name:      "ipip",
			kind:      "ipip",
			src:       `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (ipv4 :src "10.30.0.1/30")))`,
			peerInner: "10.30.0.2/30",
			target:    "10.30.0.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := nettest.NewPair(t, tt.name, "192.168.1.10", "192.168.1.20")
			p.A.RequireType(tt.kind)

			near := parse(t, tt.src)
			if err := p.A.RunScript(lowerTree(t, near, "wire0")); err != nil {
				t.Fatalf("near end failed:\n%v", err)
			}

			far, err := mirror.Peer(parse(t, tt.src), tt.peerInner)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.B.RunScript(lowerTree(t, far.Tree, "wire0")); err != nil {
				t.Fatalf("far end failed:\n%v", err)
			}

			// The prediction comes from the same tree that produced
			// both scripts.
			sp, err := spine.FromForm(near)
			if err != nil {
				t.Fatal(err)
			}
			exp, err := wire.Expect(sp)
			if err != nil {
				t.Fatal(err)
			}

			cap := p.B.StartCapture("wire0", exp.Filter, 4)
			if err := p.A.SendUDP(tt.target, 9999, 4); err != nil {
				t.Fatalf("could not generate traffic: %v", err)
			}
			captured := cap.Wait(8 * time.Second)

			if firstLines(captured, 1) == "" {
				t.Fatalf("nothing matched %q on the underlay; the tunnel carried no traffic\n%s",
					exp.Filter, captured)
			}
			for _, missing := range exp.Check(captured) {
				t.Errorf("the tree says %s is %s, but the wire does not show it (looking for %q)\n--- captured ---\n%s",
					missing.Label, missing.Value, missing.Match, firstLines(captured, 4))
			}
			if !t.Failed() {
				t.Logf("wire agrees with the tree:\n%s", firstLines(captured, 1))
			}
		})
	}
}

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
