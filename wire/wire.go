// Package wire derives, from a packet tree, what should appear on the
// underlay once the configuration built from that same tree is running.
//
// This is the third reading of one description. The first lowers it to
// commands that configure a device. The second turns it around to
// configure the far end. This one says what the wire should look like
// afterwards, which makes the two checkable against each other: bring
// the device up, capture the underlay, and compare.
//
// Nothing else in pktc closes that loop. A golden test proves the right
// commands were emitted; a kernel test proves the device was created.
// Only this says the device does what the tree claimed it would.
package wire

import (
	"fmt"
	"strings"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/spine"
)

// Fact is one thing the tree says must be visible on the underlay.
type Fact struct {
	Label string // what it is, for a human
	Value string // what the tree said
	Match string // substring that must appear in a tcpdump line
}

// Expectation is the whole of what a tree predicts.
type Expectation struct {
	Filter string // pcap filter narrowing the capture
	Facts  []Fact
}

func (e Expectation) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "filter: %s\n\nexpect on the underlay:\n", e.Filter)
	width := 0
	for _, f := range e.Facts {
		if len(f.Label) > width {
			width = len(f.Label)
		}
	}
	for _, f := range e.Facts {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, f.Label, f.Value)
	}
	return b.String()
}

// Expect reads a spine and predicts the outer headers.
//
// Only concrete fields produce facts. A '*' says the field varies
// across the packets this device carries, so there is nothing to
// assert about any one of them.
func Expect(sp spine.Spine) (Expectation, error) {
	if sp.Len() == 0 {
		return Expectation{}, fmt.Errorf("empty spine")
	}
	var e Expectation

	outer := sp.At(0)
	if outer.Name != "ipv4" && outer.Name != "ipv6" {
		return Expectation{}, fmt.Errorf(
			"the outermost layer is (%s ...); only IP underlays can be predicted", outer.Name)
	}
	src, err := ast.Get(outer, ast.Src)
	if err != nil {
		return Expectation{}, err
	}
	if v, ok := src.Get(); ok {
		e.Facts = append(e.Facts, Fact{"outer source", v.String(), v.String() + "."})
	}
	dst, err := ast.Get(outer, ast.Dst)
	if err != nil {
		return Expectation{}, err
	}
	dstAddr := ""
	if v, ok := dst.Get(); ok {
		dstAddr = v.String()
	}

	// The layer under the outer IP header decides the shape.
	if sp.Len() < 2 {
		e.Filter = "ip"
		return e, nil
	}
	switch next := sp.At(1); next.Name {
	case "udp":
		port, err := ast.Get(next, ast.DstPort)
		if err != nil {
			return Expectation{}, err
		}
		p, ok := port.Get()
		if !ok {
			return Expectation{}, fmt.Errorf(
				"(udp ...): a dynamic or absent :dst-port cannot be predicted")
		}
		e.Filter = fmt.Sprintf("udp port %d", p)
		e.Facts = append(e.Facts, Fact{"UDP destination port", fmt.Sprint(p), fmt.Sprint(p)})
		if dstAddr != "" {
			e.Facts = append(e.Facts, Fact{
				"outer destination", dstAddr,
				fmt.Sprintf("> %s.%d", dstAddr, p)})
		}
		if sp.Len() > 2 {
			if err := tunnelFacts(&e, sp.At(2)); err != nil {
				return Expectation{}, err
			}
		}
	case "gre":
		e.Filter = "proto gre"
		e.Facts = append(e.Facts, Fact{"encapsulation", "GRE", "GREv0"})
		if dstAddr != "" {
			e.Facts = append(e.Facts, Fact{"outer destination", dstAddr, "> " + dstAddr})
		}
	case "ipv4":
		e.Filter = "proto 4"
		e.Facts = append(e.Facts, Fact{"encapsulation", "IP-in-IP", "IP"})
		if dstAddr != "" {
			e.Facts = append(e.Facts, Fact{"outer destination", dstAddr, "> " + dstAddr})
		}
	default:
		return Expectation{}, fmt.Errorf(
			"(%s ...) under the outer header has no wire prediction yet", next.Name)
	}
	return e, nil
}

func tunnelFacts(e *Expectation, n *ast.Node) error {
	switch n.Name {
	case "vxlan":
		e.Facts = append(e.Facts, Fact{"encapsulation", "VXLAN", "VXLAN"})
		vni, err := ast.Get(n, ast.VNI)
		if err != nil {
			return err
		}
		if v, ok := vni.Get(); ok {
			e.Facts = append(e.Facts, Fact{"VXLAN VNI", fmt.Sprint(v), fmt.Sprintf("vni %d", v)})
		}
	case "geneve":
		e.Facts = append(e.Facts, Fact{"encapsulation", "Geneve", "Geneve"})
	}
	return nil
}

// Check reports which facts are missing from captured output.
func (e Expectation) Check(captured string) []Fact {
	var missing []Fact
	for _, f := range e.Facts {
		if !strings.Contains(captured, f.Match) {
			missing = append(missing, f)
		}
	}
	return missing
}
