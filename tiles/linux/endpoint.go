package linux

import (
	"net/netip"
	"strings"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/sexp"
)

// endpoint is one end of a tunnel as the tree describes it.
//
// A layer's :src can carry a prefix length, and on an intermediate
// layer that means two things at once: the bare address is this
// tunnel's local endpoint, and the full prefix is an address that must
// exist on the device below before this tunnel can be built on it.
//
// That is how a nested stack addresses its middle. The outermost layer
// normally names a bare address, because the physical device is already
// configured and is not ours to change.
type endpoint struct {
	Addr    string // bare address, empty when Dynamic or Absent
	Prefix  string // address/length to place on the underlay, if given
	Dynamic bool
	Absent  bool
	Pos     sexp.Pos
}

func readEndpoint(n *ast.Node, key string) (endpoint, error) {
	p, ok := n.Lookup(key)
	if !ok {
		return endpoint{Absent: true, Pos: n.Pos}, nil
	}
	if p.Val.Kind == sexp.KStar {
		return endpoint{Dynamic: true, Pos: p.Val.Pos}, nil
	}
	e := endpoint{Pos: p.Val.Pos}
	if strings.Contains(p.Val.Text, "/") {
		pfx, err := ast.ParsePrefix(p.Val)
		if err != nil {
			return endpoint{}, sexp.Errorf(p.Val.Pos, "(%s ...): :%s: %v", n.Name, key, err)
		}
		e.Addr = pfx.Addr().String()
		e.Prefix = pfx.String()
		return e, nil
	}
	a, err := ast.ParseAddr(p.Val)
	if err != nil {
		return endpoint{}, sexp.Errorf(p.Val.Pos, "(%s ...): :%s: %v", n.Name, key, err)
	}
	e.Addr = a.String()
	return e, nil
}

// IsMulticast reports whether a concrete address is a multicast group.
func (e endpoint) IsMulticast() bool {
	if e.Addr == "" {
		return false
	}
	a, err := netip.ParseAddr(e.Addr)
	return err == nil && a.IsMulticast()
}

// underlayAddress emits the address this layer needs on the device
// below, if the tree gave one. Nothing is emitted for a bare address:
// that names an endpoint without claiming the device already has it.
func (e endpoint) underlayAddress(dev string) command.Script {
	if e.Prefix == "" {
		return nil
	}
	return command.Script{command.New(
		"the address this tunnel is built on must exist on "+dev,
		"ip", "addr", "add", e.Prefix, "dev", dev)}
}
