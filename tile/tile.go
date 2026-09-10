// Package tile implements instruction selection over an encapsulation
// spine.
//
// Each tile recognises a run of consecutive protocol layers and knows
// how to configure the device that produces them. Because a spine does
// not branch, covering it is segmentation of a list rather than a tree
// cover, and greedy longest-match is optimal.
package tile

import (
	"fmt"
	"strings"

	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/spine"
)

// Tile recognises one shape and lowers it.
//
// Pattern matches on protocol names; Guard checks field values. Keeping
// those separate is what makes the matcher cheap.
type Tile struct {
	Name     string
	Pattern  []string
	Priority int // lower wins a tie; only consulted for identical patterns
	Guard    func(spine.Spine) bool
	Lower    func(spine.Spine, *env.Env) (command.Script, error)
}

func (t Tile) String() string { return strings.Join(t.Pattern, " · ") }

// Registry is the tile set for one target.
type Registry struct {
	Target string
	tiles  []Tile
}

// NewRegistry validates a tile set at load time. Two tiles with the same
// pattern and the same priority cannot be resolved deterministically, so
// that is a startup error rather than a surprise during lowering.
func NewRegistry(target string, tiles ...Tile) (*Registry, error) {
	seen := map[string]string{}
	for _, t := range tiles {
		if len(t.Pattern) == 0 {
			return nil, fmt.Errorf("tile %s has an empty pattern", t.Name)
		}
		if t.Lower == nil {
			return nil, fmt.Errorf("tile %s has no Lower function", t.Name)
		}
		k := fmt.Sprintf("%s|%d", strings.Join(t.Pattern, "\x00"), t.Priority)
		if prev, dup := seen[k]; dup {
			return nil, fmt.Errorf(
				"tiles %s and %s share pattern %s at priority %d; give one a distinct priority",
				prev, t.Name, t, t.Priority)
		}
		seen[k] = t.Name
	}
	return &Registry{Target: target, tiles: tiles}, nil
}

// Tiles returns the registered tiles.
func (r *Registry) Tiles() []Tile { return r.tiles }

// Names returns the tile names, for diagnostics.
func (r *Registry) Names() []string {
	out := make([]string, len(r.tiles))
	for i, t := range r.tiles {
		out[i] = t.Name
	}
	return out
}

// matchAt reports every tile that matches the spine starting at i.
func (r *Registry) matchAt(sp spine.Spine, i int) []Tile {
	var out []Tile
	for _, t := range r.tiles {
		if i+len(t.Pattern) > sp.Len() {
			continue
		}
		ok := true
		for k, want := range t.Pattern {
			if sp.At(i+k).Name != want {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		sub := sp.Slice(i, i+len(t.Pattern))
		if t.Guard != nil && !t.Guard(sub) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// best picks the longest match, breaking ties on the lower priority.
func best(cands []Tile) Tile {
	pick := cands[0]
	for _, t := range cands[1:] {
		switch {
		case len(t.Pattern) > len(pick.Pattern):
			pick = t
		case len(t.Pattern) == len(pick.Pattern) && t.Priority < pick.Priority:
			pick = t
		}
	}
	return pick
}
