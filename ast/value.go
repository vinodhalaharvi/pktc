package ast

import "github.com/vinodhalaharvi/pktc/sexp"

// State is the three-way distinction the whole design rests on: a
// property is either absent, dynamic ('*', meaning it varies across the
// packets a device handles), or concrete.
//
// These are not interchangeable. A concrete value pins a device
// parameter; '*' selects a different kernel mode; absence is an error
// wherever the parameter is required.
type State uint8

const (
	Absent State = iota
	Dynamic
	Concrete
)

func (s State) String() string {
	switch s {
	case Absent:
		return "absent"
	case Dynamic:
		return "dynamic"
	case Concrete:
		return "concrete"
	}
	return "unknown"
}

// Value is a typed property reading. T never becomes any on its way to
// the emitted command.
type Value[T any] struct {
	state State
	val   T
	pos   sexp.Pos
	key   string
}

func (v Value[T]) State() State    { return v.state }
func (v Value[T]) IsAbsent() bool  { return v.state == Absent }
func (v Value[T]) IsDynamic() bool { return v.state == Dynamic }
func (v Value[T]) Pos() sexp.Pos   { return v.pos }
func (v Value[T]) Key() string     { return v.key }

// Get returns the value and whether it is concrete.
func (v Value[T]) Get() (T, bool) { return v.val, v.state == Concrete }

// Require returns a concrete value, or an error explaining which of the
// two other states was found. node names the form for the diagnostic.
func (v Value[T]) Require(node string) (T, error) {
	var zero T
	switch v.state {
	case Concrete:
		return v.val, nil
	case Dynamic:
		return zero, sexp.Errorf(v.pos,
			"(%s ...): :%s must be a concrete value here, not '*'", node, v.key)
	default:
		return zero, sexp.Errorf(v.pos,
			"(%s ...): missing required property :%s", node, v.key)
	}
}

// PropDef names a property and says how to parse its value.
type PropDef[T any] struct {
	Key   string
	Parse func(sexp.Value) (T, error)
}

// Get reads one typed property off a node.
func Get[T any](n *Node, def PropDef[T]) (Value[T], error) {
	p, ok := n.Lookup(def.Key)
	if !ok {
		return Value[T]{state: Absent, pos: n.Pos, key: def.Key}, nil
	}
	if p.Val.Kind == sexp.KStar {
		return Value[T]{state: Dynamic, pos: p.Val.Pos, key: def.Key}, nil
	}
	t, err := def.Parse(p.Val)
	if err != nil {
		return Value[T]{}, sexp.Errorf(p.Val.Pos, "(%s ...): :%s: %v", n.Name, def.Key, err)
	}
	return Value[T]{state: Concrete, val: t, pos: p.Val.Pos, key: def.Key}, nil
}
