package linux

import (
	"strconv"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
	"github.com/vinodhalaharvi/pktc/tile"
)

// VXLAN recognises Ethernet carried inside VXLAN over UDP.
//
// This is the tile where one packet stops being enough. A packet
// describes a single frame between two endpoints; a device serves a
// whole class of frames. Where the two disagree, '*' says which field
// varies, and each choice selects a different kernel mode rather than
// relaxing a constraint.
var VXLAN = tile.Tile{
	Name:    "VXLAN",
	Pattern: []string{"ipv4", "udp", "vxlan", "ethernet"},
	Lower:   lowerVXLAN,
}

func lowerVXLAN(sp spine.Spine, e *env.Env) (command.Script, error) {
	outer, transport, tunnel := sp.At(0), sp.At(1), sp.At(2)

	vni, err := ast.Get(tunnel, ast.VNI)
	if err != nil {
		return nil, err
	}
	if vni.IsAbsent() {
		return nil, sexp.Errorf(tunnel.Pos,
			"(vxlan ...): missing required property :vni; give a VNI, "+
				"or '*' for one device serving every VNI")
	}

	port, err := ast.Get(transport, ast.DstPort)
	if err != nil {
		return nil, err
	}
	if port.IsDynamic() {
		return nil, sexp.Errorf(port.Pos(),
			"(udp ...): :dst-port '*' has no lowering; a VXLAN device listens on one port")
	}
	if port.IsAbsent() {
		// Not a default worth guessing: the kernel picks 8472, the
		// pre-standard value, while the IANA assignment is 4789. A
		// device built on the wrong one comes up cleanly and carries
		// nothing.
		return nil, sexp.Errorf(transport.Pos,
			"(udp ...): missing required property :dst-port; pktc will not guess, "+
				"because the kernel default is 8472 and the IANA value is 4789")
	}
	p, _ := port.Get()

	local, err := ast.Get(outer, ast.Src)
	if err != nil {
		return nil, err
	}
	if local.IsAbsent() {
		return nil, sexp.Errorf(outer.Pos,
			"(%s ...): missing required property :src; give the underlay address, "+
				"or '*' to let the kernel choose it from the device", outer.Name)
	}

	remote, err := ast.Get(outer, ast.Dst)
	if err != nil {
		return nil, err
	}
	if remote.IsAbsent() {
		return nil, sexp.Errorf(outer.Pos,
			"(%s ...): missing required property :dst; give the peer address, a multicast "+
				"group, or '*' when remotes are learned rather than configured", outer.Name)
	}

	// Naming: a concrete VNI names the device after itself, which is
	// both readable and collision-free across rules. A metadata-driven
	// device has no VNI to name it after.
	var dev string
	if n, ok := vni.Get(); ok {
		dev, err = e.Claim("vxlan" + strconv.FormatUint(uint64(n), 10))
	} else {
		dev, err = e.Alloc("vxlan")
	}
	if err != nil {
		return nil, sexp.Errorf(tunnel.Pos, "%v", err)
	}

	argv := []string{"ip", "link", "add", "name", dev, "type", "vxlan"}
	why := "Ethernet inside VXLAN is a vxlan device"

	if n, ok := vni.Get(); ok {
		argv = append(argv, "id", u32(n))
	} else {
		// external is collect-metadata mode: the VNI arrives with each
		// packet instead of being fixed on the device. The kernel
		// rejects 'external' together with an id, which is the same
		// exclusion the '*' states already express.
		argv = append(argv, "external")
		why += "; :vni '*' means one device for every VNI, driven by route metadata"
	}

	if l, ok := local.Get(); ok {
		argv = append(argv, "local", l.String())
	} else {
		why += "; :src '*' leaves the source address to the underlay device"
	}

	if r, ok := remote.Get(); ok {
		// A multicast outer destination is not a peer, it is the group
		// every peer joins. Same field, different kernel parameter.
		if r.IsMulticast() {
			argv = append(argv, "group", r.String())
			why += "; a multicast outer destination becomes a group rather than a remote"
		} else {
			argv = append(argv, "remote", r.String())
		}
	} else {
		why += "; :dst '*' means peers are learned rather than configured"
	}

	argv = append(argv, "dstport", u16(p), "dev", e.Dev)

	e.Enter(dev)
	return command.Script{command.New(why, argv...)}, nil
}
