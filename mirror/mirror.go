// Package mirror derives the far end of a tunnel from the near end.
//
// A tunnel is symmetric, so most of the peer's configuration is the
// local configuration with the outer source and destination exchanged.
// That symmetry is real and worth exploiting: it is the same tree, read
// from the other end, and it removes the step people most often get
// wrong by hand.
//
// The mirror is not pure, though, and the parts it cannot derive matter
// more than the parts it can. A VNI, a GRE key and an ESP SPI are
// identical at both ends. An inner address is not: both ends share a
// prefix and differ in the host part, and nothing in the near end's
// tree says which host part the peer uses. The underlay device name,
// any pre-existing interface names, and asymmetric key material are all
// facts about the far machine.
//
// So Peer swaps what it can, substitutes what it is told, and reports
// everything left over rather than inventing it.
package mirror

import (
	"fmt"
	"strings"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/sexp"
)

// Result is the peer's tree and the facts that had to come from
// somewhere other than the packet.
type Result struct {
	Tree     *ast.Node
	Supplied []string // derived or substituted, for the reader's benefit
	Missing  []string // could not be derived and were not supplied
}

// Peer builds the far end's tree.
//
// innerAddr replaces the innermost address if given. When it is empty
// the address is dropped and reported as missing, because guessing a
// host part is the kind of helpfulness that produces a tunnel which
// comes up and does not work.
func Peer(root *ast.Node, innerAddr string) (Result, error) {
	if innerAddr != "" {
		if _, err := ast.ParsePrefix(sexp.Value{Kind: sexp.KString, Text: innerAddr}); err != nil {
			return Result{}, fmt.Errorf("peer inner address: %v", err)
		}
	}

	tree := clone(root)
	res := Result{Tree: tree}

	// Mirror the packet, not the wrapper around it.
	packet := tree
	if packet.Name == "configure" && len(packet.Children) == 1 {
		packet = packet.Children[0]
	}

	layers := spineOf(packet)
	if len(layers) == 0 {
		return Result{}, fmt.Errorf("nothing to mirror")
	}

	// Every layer naming both ends is a two-ended relationship and has
	// to turn around. A nested stack has more than one: the outer
	// underlay and the address of each tunnel it stands on.
	swapped := 0
	for _, n := range layers {
		_, hasSrc := n.Lookup("src")
		_, hasDst := n.Lookup("dst")
		if !hasSrc || !hasDst {
			continue
		}
		if err := swapEndpoints(n); err != nil {
			return Result{}, err
		}
		swapped++
		res.Supplied = append(res.Supplied,
			fmt.Sprintf(":src and :dst exchanged on (%s ...)", n.Name))
	}
	if swapped == 0 {
		return Result{}, sexp.Errorf(layers[0].Pos,
			"(%s ...): mirroring needs a layer naming both :src and :dst; a tunnel has two ends",
			layers[0].Name)
	}

	// Identical at both ends, so left alone. Naming them is the point:
	// a reader should be able to see that the mirror knew to leave them.
	for _, n := range layers {
		for _, key := range []string{"vni", "key", "spi"} {
			if _, ok := n.Lookup(key); ok {
				res.Supplied = append(res.Supplied,
					fmt.Sprintf(":%s on (%s ...) is the same at both ends", key, n.Name))
			}
		}
	}

	// Some things a mirror cannot reach. A WireGuard peer holds its own
	// private key and needs the other side's public key, so the far
	// end's configuration is not a reading of the near end's tree at
	// all. This is the first asymmetry that is structural rather than
	// an address, and saying so is more useful than emitting a script
	// that looks complete and is not.
	for _, n := range layers {
		if n.Name != "wireguard" {
			continue
		}
		res.Missing = append(res.Missing, fmt.Sprintf(
			"key material on (%s ...): each end holds its own private key and needs the "+
				"other's public key, which no reading of this tree can supply", n.Name))
		if p, ok := n.Lookup("peer"); ok {
			res.Missing = append(res.Missing, fmt.Sprintf(
				":peer %q names this end's view of the far end; the far end needs a name "+
					"for this one", p.Val.Text))
		}
	}

	inner := innermostAddressed(layers)
	switch {
	case inner == nil:
		// Nothing to do: this tree carries no inner address.
	case innerAddr != "":
		setProp(inner, "src", innerAddr)
		res.Supplied = append(res.Supplied,
			fmt.Sprintf("inner address on (%s ...) set to %s", inner.Name, innerAddr))
	default:
		removeProp(inner, "src")
		res.Missing = append(res.Missing, fmt.Sprintf(
			"inner address on (%s ...): both ends share the prefix and differ in the "+
				"host part, which this tree does not contain", inner.Name))
	}

	return res, nil
}

// swapEndpoints exchanges the two ends of one layer.
//
// The addresses swap; the prefix lengths do not. A layer written as
// :src "10.100.0.1/24" :dst "10.100.0.2" mirrors to :src "10.100.0.2/24"
// :dst "10.100.0.1", because the length says how wide the shared subnet
// is and belongs to whichever field is the local one. This is the one
// place a peer's inner address can be derived rather than supplied:
// both host parts are already written down.
//
// A '*' swaps as itself, so :src X :dst * becomes :src * :dst X. The end
// that learned its peers becomes the end that is learned.
func swapEndpoints(n *ast.Node) error {
	src, _ := n.Lookup("src")
	dst, _ := n.Lookup("dst")

	srcAddr, srcLen := splitPrefix(src.Val)
	dstAddr, dstLen := splitPrefix(dst.Val)

	newSrc := rebuild(dst.Val, dstAddr, srcLen)
	newDst := rebuild(src.Val, srcAddr, dstLen)

	for i := range n.Props {
		switch n.Props[i].Key {
		case "src":
			n.Props[i].Val = newSrc
		case "dst":
			n.Props[i].Val = newDst
		}
	}
	return nil
}

// splitPrefix separates an address from its length, if it has one.
func splitPrefix(v sexp.Value) (addr, length string) {
	if v.Kind == sexp.KStar {
		return "", ""
	}
	if i := strings.IndexByte(v.Text, '/'); i >= 0 {
		return v.Text[:i], v.Text[i+1:]
	}
	return v.Text, ""
}

// rebuild puts an address back together with the length belonging to
// its new position. A '*' has no address and stays a '*'.
func rebuild(orig sexp.Value, addr, length string) sexp.Value {
	if orig.Kind == sexp.KStar || addr == "" {
		return orig
	}
	text := addr
	if length != "" {
		text = addr + "/" + length
	}
	return sexp.Value{Kind: sexp.KString, Text: text, Pos: orig.Pos}
}

// innermostAddressed finds the deepest layer carrying an address.
func innermostAddressed(layers []*ast.Node) *ast.Node {
	for i := len(layers) - 1; i >= 0; i-- {
		n := layers[i]
		switch n.Name {
		case "ipv4", "ipv6":
		default:
			continue
		}
		p, ok := n.Lookup("src")
		if !ok {
			continue
		}
		if _, err := ast.ParsePrefix(p.Val); err == nil {
			return n
		}
	}
	return nil
}

func setProp(n *ast.Node, key, text string) {
	for i := range n.Props {
		if n.Props[i].Key == key {
			n.Props[i].Val = sexp.Value{Kind: sexp.KString, Text: text, Pos: n.Props[i].Val.Pos}
			return
		}
	}
	n.Props = append(n.Props, ast.Prop{
		Key: key,
		Val: sexp.Value{Kind: sexp.KString, Text: text, Pos: n.Pos},
		Pos: n.Pos,
	})
}

func removeProp(n *ast.Node, key string) {
	out := n.Props[:0]
	for _, p := range n.Props {
		if p.Key != key {
			out = append(out, p)
		}
	}
	n.Props = out
}

// spineOf collects the single-child chain below root, including root.
func spineOf(root *ast.Node) []*ast.Node {
	var out []*ast.Node
	for n := root; n != nil; {
		out = append(out, n)
		if len(n.Children) != 1 {
			break
		}
		n = n.Children[0]
	}
	return out
}

func clone(n *ast.Node) *ast.Node {
	if n == nil {
		return nil
	}
	c := &ast.Node{Name: n.Name, Pos: n.Pos}
	c.Props = append(c.Props, n.Props...)
	for _, ch := range n.Children {
		c.Children = append(c.Children, clone(ch))
	}
	return c
}
