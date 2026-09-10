// Package ast turns generic S-expression values into protocol nodes.
//
// A form is (name :prop value ... (child) ...): a protocol name, then
// zero or more properties, then zero or more nested forms. Properties
// must precede nested forms.
package ast

import "github.com/vinodhalaharvi/pktc/sexp"

// Prop is one :key value pair on a node.
type Prop struct {
	Key string     // without the leading colon
	Val sexp.Value // the raw value; Kind distinguishes '*' from a literal
	Pos sexp.Pos   // position of the key
}

// Node is one protocol layer in a packet tree.
type Node struct {
	Name     string
	Props    []Prop
	Children []*Node
	Pos      sexp.Pos
}

// Lookup finds a property by key. Nodes carry a handful of properties,
// so a linear scan is the right structure here.
func (n *Node) Lookup(key string) (Prop, bool) {
	for _, p := range n.Props {
		if p.Key == key {
			return p, true
		}
	}
	return Prop{}, false
}

// Build converts one generic form into a Node tree.
func Build(v sexp.Value) (*Node, error) {
	if v.Kind != sexp.KList {
		return nil, sexp.Errorf(v.Pos, "expected a form '(...)', got %s", v.Kind)
	}
	if len(v.Items) == 0 {
		return nil, sexp.Errorf(v.Pos, "empty form '()'")
	}
	head := v.Items[0]
	if head.Kind != sexp.KSymbol {
		return nil, sexp.Errorf(head.Pos, "a form must start with a protocol name, got %s", head.Kind)
	}

	n := &Node{Name: head.Text, Pos: v.Pos}

	i := 1
	for i < len(v.Items) && v.Items[i].Kind == sexp.KKeyword {
		kw := v.Items[i]
		if i+1 >= len(v.Items) {
			return nil, sexp.Errorf(kw.Pos, "property %s has no value", kw.Text)
		}
		val := v.Items[i+1]
		if val.Kind == sexp.KKeyword {
			return nil, sexp.Errorf(kw.Pos, "property %s has no value", kw.Text)
		}
		key := kw.Key()
		if _, dup := n.Lookup(key); dup {
			return nil, sexp.Errorf(kw.Pos, "duplicate property %s on (%s ...)", kw.Text, n.Name)
		}
		n.Props = append(n.Props, Prop{Key: key, Val: val, Pos: kw.Pos})
		i += 2
	}

	for ; i < len(v.Items); i++ {
		item := v.Items[i]
		if item.Kind == sexp.KKeyword {
			return nil, sexp.Errorf(item.Pos,
				"property %s must come before any nested form", item.Text)
		}
		if item.Kind != sexp.KList {
			return nil, sexp.Errorf(item.Pos, "expected a nested form '(...)', got %s", item.Kind)
		}
		c, err := Build(item)
		if err != nil {
			return nil, err
		}
		n.Children = append(n.Children, c)
	}
	return n, nil
}
