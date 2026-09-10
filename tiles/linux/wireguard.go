package linux

import (
	"fmt"
	"net/netip"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
	"github.com/vinodhalaharvi/pktc/tile"
)

// KeyDir is where the emitted script looks for key material.
//
// Keys are not packet structure and must never appear in a tree: a
// packet description is something people paste into issues. The tree
// names a peer; the name resolves to a file on the machine.
const KeyDir = "/etc/wireguard"

// WireGuard recognises IP carried inside WireGuard over UDP.
//
// Every other tile so far has been symmetric: the far end is the near
// end with its endpoints exchanged. This one is not, and it is the
// first place the mirror has to admit that. Each side holds its own
// private key and needs the other's public key, so no amount of reading
// one end's tree produces the other end's configuration.
var WireGuard = tile.Tile{
	Name:    "WireGuard",
	Pattern: []string{"ipv4", "udp", "wireguard", "ipv4"},
	Lower:   lowerWireGuard,
}

func lowerWireGuard(sp spine.Spine, e *env.Env) (command.Script, error) {
	outer, transport, tunnel, inner := sp.At(0), sp.At(1), sp.At(2), sp.At(3)

	peer, err := ast.Get(tunnel, ast.Peer)
	if err != nil {
		return nil, err
	}
	peerName, err := peer.Require("wireguard")
	if err != nil {
		return nil, sexp.Errorf(tunnel.Pos,
			"(wireguard ...): missing required property :peer; name the peer, and the "+
				"name resolves to key material on the machine. Keys are not packet structure")
	}

	listen, err := ast.Get(transport, ast.SrcPort)
	if err != nil {
		return nil, err
	}
	port, err := listen.Require("udp")
	if err != nil {
		return nil, sexp.Errorf(transport.Pos,
			"(udp ...): missing required property :src-port; WireGuard listens on a "+
				"fixed port, which is the port the peer sends to")
	}

	endpoint, err := readEndpoint(outer, "dst")
	if err != nil {
		return nil, err
	}
	epPort, err := ast.Get(transport, ast.DstPort)
	if err != nil {
		return nil, err
	}
	remotePort, ok := epPort.Get()
	if !ok {
		remotePort = port // a symmetric pair is the common case
	}

	// allowed-ips comes from the inner prefix: the subnet this tunnel
	// carries is exactly the range the peer is permitted to use.
	pfx, err := ast.Get(inner, ast.SrcPrefix)
	if err != nil {
		return nil, err
	}
	overlay, err := pfx.Require(inner.Name)
	if err != nil {
		return nil, sexp.Errorf(inner.Pos,
			"(%s ...): missing required property :src; the inner prefix is what "+
				"allowed-ips is derived from", inner.Name)
	}

	dev, err := e.Alloc("wg")
	if err != nil {
		return nil, sexp.Errorf(tunnel.Pos, "%v", err)
	}

	out := command.Script{
		command.New("IP inside WireGuard is a wireguard device",
			"ip", "link", "add", "name", dev, "type", "wireguard"),
		command.New("key material is machine state, not packet structure",
			"wg", "set", dev, "listen-port", u16(port),
			"private-key", fmt.Sprintf("%s/%s.key", KeyDir, dev)),
	}

	argv := []string{"wg", "set", dev, "peer",
		fmt.Sprintf("$(cat %s/%s.pub)", KeyDir, peerName)}
	if endpoint.Dynamic || endpoint.Absent {
		// A peer behind NAT cannot be addressed; it announces itself on
		// first handshake. WireGuard is the only tile here that can do
		// this, which is why :dst '*' has a lowering at all.
		out = append(out, command.New(
			"no endpoint: this peer is learned from its first handshake",
			append(argv, "allowed-ips", overlay.Masked().String())...))
	} else {
		out = append(out, command.New(
			"the peer's endpoint on the underlay",
			append(argv,
				"endpoint", joinHostPort(endpoint.Addr, remotePort),
				"allowed-ips", overlay.Masked().String())...))
	}

	e.Enter(dev)

	// The tile consumes the inner layer, so the address on the tunnel
	// is this tile's to emit rather than the cover's.
	addr, err := addrOn(inner, dev)
	if err != nil {
		return nil, err
	}
	return append(out, addr...), nil
}

func joinHostPort(addr string, port uint16) string {
	if a, err := netip.ParseAddr(addr); err == nil && a.Is6() {
		return fmt.Sprintf("[%s]:%d", addr, port)
	}
	return fmt.Sprintf("%s:%d", addr, port)
}
