package linux

import (
	"fmt"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
	"github.com/vinodhalaharvi/pktc/tile"
)

// VLAN recognises an 802.1Q tag inside an Ethernet frame.
//
// This is the only tile whose outermost layer is Ethernet rather than
// IP, and it is usually the first of a chain: a tagged interface is
// something other tunnels are then built on. The kernel names such a
// device after its parent, so the naming follows suit.
var VLAN = tile.Tile{
	Name:    "VLAN",
	Pattern: []string{"ethernet", "vlan"},
	Lower: func(sp spine.Spine, e *env.Env) (command.Script, error) {
		tag := sp.At(1)

		id, err := ast.Get(tag, ast.VLANID)
		if err != nil {
			return nil, err
		}
		if id.IsDynamic() {
			return nil, sexp.Errorf(id.Pos(),
				"(vlan ...): :id '*' has no lowering; a tagged interface carries one tag. "+
					"An interface that carries every tag is the untagged parent itself")
		}
		n, err := id.Require("vlan")
		if err != nil {
			return nil, err
		}

		dev, err := e.Claim(fmt.Sprintf("%s.%d", e.Dev, n))
		if err != nil {
			return nil, sexp.Errorf(tag.Pos, "%v", err)
		}

		out := command.Script{command.New(
			"an 802.1Q tag inside Ethernet is a tagged interface on its parent",
			"ip", "link", "add", "link", e.Dev, "name", dev, "type", "vlan", "id", u16(n))}

		e.Enter(dev)

		// A tagged interface can be addressed directly, or used as the
		// underlay for whatever the tree nests below it.
		if len(sp.Layers) > 2 {
			return out, nil
		}
		return out, nil
	},
}
