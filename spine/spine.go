// Package spine flattens a packet tree into the linear stack the tiler
// walks.
//
// Encapsulation does not branch: each layer carries exactly one thing.
// That is why tiling here is segmentation of a list rather than a tree
// cover, and why greedy longest-match is optimal.
package spine

import (
	"strings"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/sexp"
)

// Spine is the outermost-to-innermost sequence of protocol layers.
type Spine struct {
	Layers []*ast.Node
}

func (s Spine) Len() int           { return len(s.Layers) }
func (s Spine) At(i int) *ast.Node { return s.Layers[i] }

func (s Spine) Names() []string {
	out := make([]string, len(s.Layers))
	for i, n := range s.Layers {
		out[i] = n.Name
	}
	return out
}

// String renders the spine the way diagnostics refer to it.
func (s Spine) String() string { return strings.Join(s.Names(), " · ") }

// Slice returns the sub-spine [i, j).
func (s Spine) Slice(i, j int) Spine { return Spine{Layers: s.Layers[i:j]} }

// Extract walks a packet tree down to its innermost layer.
func Extract(root *ast.Node) (Spine, error) {
	var layers []*ast.Node
	for n := root; ; {
		layers = append(layers, n)
		switch len(n.Children) {
		case 0:
			return Spine{Layers: layers}, nil
		case 1:
			n = n.Children[0]
		default:
			return Spine{}, sexp.Errorf(n.Children[1].Pos,
				"(%s ...) contains %d nested forms; an encapsulation stack carries exactly one",
				n.Name, len(n.Children))
		}
	}
}

// ConfigureForm is the top-level form this tool compiles.
const ConfigureForm = "configure"

// FromForm validates a (configure ...) wrapper and extracts its spine.
func FromForm(root *ast.Node) (Spine, error) {
	if root.Name != ConfigureForm {
		return Spine{}, sexp.Errorf(root.Pos,
			"expected a (%s ...) form, found (%s ...)", ConfigureForm, root.Name)
	}
	if len(root.Props) > 0 {
		return Spine{}, sexp.Errorf(root.Props[0].Pos,
			"(%s ...) takes no properties", ConfigureForm)
	}
	if len(root.Children) != 1 {
		return Spine{}, sexp.Errorf(root.Pos,
			"(%s ...) must contain exactly one packet form, found %d",
			ConfigureForm, len(root.Children))
	}
	return Extract(root.Children[0])
}
