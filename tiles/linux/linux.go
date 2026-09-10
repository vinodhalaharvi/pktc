// Package linux lowers packet shapes to iproute2 commands.
//
// The patterns live here alongside the emission, but the shapes
// themselves are facts about packets: ipv4 · gre · ipv4 is the same run
// of layers whatever tool configures it. Only Lower is target-specific.
package linux

import (
	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
	"github.com/vinodhalaharvi/pktc/tile"
)

// Target names this backend.
const Target = "linux-iproute2"

// Registry returns the iproute2 tile set.
func Registry() (*tile.Registry, error) {
	return tile.NewRegistry(Target, IPIP, GRE, GRETAP, VXLAN, VLAN, WireGuard)
}

// endpoints reads the outer local and remote addresses.
//
// Neither may be '*' for these tiles: a point-to-point tunnel device
// carries exactly one remote, so a dynamic destination has no lowering
// here rather than a different one.
func endpoints(outer *ast.Node) (local, remote string, err error) {
	src, err := ast.Get(outer, ast.Src)
	if err != nil {
		return "", "", err
	}
	l, err := src.Require(outer.Name)
	if err != nil {
		return "", "", err
	}
	dst, err := ast.Get(outer, ast.Dst)
	if err != nil {
		return "", "", err
	}
	if dst.IsDynamic() {
		return "", "", sexp.Errorf(dst.Pos(),
			"(%s ...): :dst '*' has no lowering for a point-to-point tunnel; "+
				"only VXLAN-style devices serve many remotes", outer.Name)
	}
	rr, err := dst.Require(outer.Name)
	if err != nil {
		return "", "", err
	}
	return l.String(), rr.String(), nil
}

// addrOn emits an address command for a layer that carries one.
func addrOn(n *ast.Node, dev string) (command.Script, error) {
	addr, err := tile.AddressOf(n)
	if err != nil || addr == "" {
		return nil, err
	}
	return command.Script{
		command.New("address on the tunnel", "ip", "addr", "add", addr, "dev", dev),
	}, nil
}

// IPIP recognises IPv4 carried directly inside IPv4.
var IPIP = tile.Tile{
	Name:    "IPIP",
	Pattern: []string{"ipv4", "ipv4"},
	Lower: func(sp spine.Spine, e *env.Env) (command.Script, error) {
		local, remote, err := endpoints(sp.At(0))
		if err != nil {
			return nil, err
		}
		dev, err := e.Alloc("ipip")
		if err != nil {
			return nil, err
		}
		out := command.Script{command.New(
			"IPv4 inside IPv4 is an ipip device",
			"ip", "link", "add", "name", dev, "type", "ipip",
			"local", local, "remote", remote)}
		e.Enter(dev)

		addr, err := addrOn(sp.At(1), dev)
		if err != nil {
			return nil, err
		}
		return append(out, addr...), nil
	},
}

// GRE recognises IPv4 inside GRE: a routed tunnel.
var GRE = tile.Tile{
	Name:    "GRE",
	Pattern: []string{"ipv4", "gre", "ipv4"},
	Lower: func(sp spine.Spine, e *env.Env) (command.Script, error) {
		local, remote, err := endpoints(sp.At(0))
		if err != nil {
			return nil, err
		}
		dev, err := e.Alloc("gre")
		if err != nil {
			return nil, err
		}
		argv := []string{"ip", "link", "add", "name", dev, "type", "gre",
			"local", local, "remote", remote}

		key, err := ast.Get(sp.At(1), ast.Key)
		if err != nil {
			return nil, err
		}
		if k, ok := key.Get(); ok {
			argv = append(argv, "key", u32(k))
		}

		out := command.Script{command.New(
			"IPv4 inside GRE is a routed gre device", argv...)}
		e.Enter(dev)

		addr, err := addrOn(sp.At(2), dev)
		if err != nil {
			return nil, err
		}
		return append(out, addr...), nil
	},
}

// GRETAP recognises Ethernet inside GRE: a bridged tunnel.
//
// Same first two layers as GRE. The inner layer decides which kernel
// device is required, which is the L2-over-L3 versus L3-over-L3
// distinction doing real work.
var GRETAP = tile.Tile{
	Name:    "GRETAP",
	Pattern: []string{"ipv4", "gre", "ethernet"},
	Lower: func(sp spine.Spine, e *env.Env) (command.Script, error) {
		local, remote, err := endpoints(sp.At(0))
		if err != nil {
			return nil, err
		}
		dev, err := e.Alloc("gretap")
		if err != nil {
			return nil, err
		}
		argv := []string{"ip", "link", "add", "name", dev, "type", "gretap",
			"local", local, "remote", remote}

		key, err := ast.Get(sp.At(1), ast.Key)
		if err != nil {
			return nil, err
		}
		if k, ok := key.Get(); ok {
			argv = append(argv, "key", u32(k))
		}

		e.Enter(dev)
		return command.Script{command.New(
			"Ethernet inside GRE is a bridged gretap device", argv...)}, nil
	},
}
