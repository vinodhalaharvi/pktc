package tile

import (
	"strings"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
)

// Result is the outcome of covering a spine.
type Result struct {
	Script  command.Script
	Applied []string    // tile names, outermost first
	Payload spine.Spine // layers below the configuration: traffic, not config
}

// Cover tiles a spine with greedy longest-match.
//
// Layers that no tile claims are not an error once configuration has
// begun: they are the traffic that flows through the device. A leftover
// layer carrying an address becomes an address on the innermost device.
func (r *Registry) Cover(sp spine.Spine, e *env.Env) (Result, error) {
	var res Result
	created := len(e.Created) // rules before this one are already up
	i := 0
	for i < sp.Len() {
		cands := r.matchAt(sp, i)
		if len(cands) == 0 {
			break
		}
		t := best(cands)
		sub := sp.Slice(i, i+len(t.Pattern))
		cmds, err := t.Lower(sub, e)
		if err != nil {
			return Result{}, err
		}
		res.Script = append(res.Script, cmds...)
		res.Applied = append(res.Applied, t.Name)
		i += len(t.Pattern)
	}

	if i == 0 {
		return Result{}, sexp.Errorf(sp.At(0).Pos,
			"no lowering exists for %s on target %s\n       tiles available: %s",
			sp, r.Target, strings.Join(r.Names(), ", "))
	}

	rest := sp.Slice(i, sp.Len())
	if rest.Len() > 0 {
		addr, err := AddressOf(rest.At(0))
		if err != nil {
			return Result{}, err
		}
		if addr != "" {
			res.Script = append(res.Script,
				command.New("address on the tunnel", "ip", "addr", "add", addr, "dev", e.Dev))
			rest = rest.Slice(1, rest.Len())
		}
	}
	res.Payload = rest

	// Every device in the chain has to come up, outermost first: an
	// inner tunnel cannot carry traffic over an underlay that is down.
	for _, dev := range e.Created[created:] {
		res.Script = append(res.Script,
			command.New("bring "+dev+" up", "ip", "link", "set", dev, "up"))
	}
	return res, nil
}

// AddressOf returns the address/length carried by an L3 layer, or "" if
// the layer carries no address and is therefore traffic rather than
// configuration.
func AddressOf(n *ast.Node) (string, error) {
	switch n.Name {
	case "ipv4", "ipv6":
	default:
		return "", nil
	}
	if _, ok := n.Lookup("src"); !ok {
		return "", nil
	}
	p, err := ast.Get(n, ast.SrcPrefix)
	if err != nil {
		return "", err
	}
	v, ok := p.Get()
	if !ok {
		return "", nil
	}
	return v.String(), nil
}
