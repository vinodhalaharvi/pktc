package tile

import (
	"fmt"
	"strings"

	"github.com/vinodhalaharvi/pktc/spine"
)

// Attempt records one tile's bid at one position, and why it lost.
type Attempt struct {
	Name     string
	Pattern  string
	Length   int
	Priority int
	Matched  bool
	Reason   string // why it did not match
}

// Step is the decision made at one position in the spine.
type Step struct {
	At       int
	Covered  string
	Chosen   string
	Because  string
	Attempts []Attempt
}

// Explanation is the whole covering decision, in the order it was made.
type Explanation struct {
	Spine    spine.Spine
	Target   string
	Steps    []Step
	Leftover spine.Spine
	Note     string
}

// Explain walks the same algorithm Cover does and records every bid
// rather than emitting commands.
//
// Instruction selection is easy to trust when it works and opaque when
// it does not. This is the view that makes the difference between "no
// tile matched" and "a tile nearly matched, and here is the layer that
// stopped it".
func (r *Registry) Explain(sp spine.Spine) Explanation {
	ex := Explanation{Spine: sp, Target: r.Target}
	i := 0
	for i < sp.Len() {
		step := Step{At: i}
		var winners []Tile
		for _, t := range r.tiles {
			a := Attempt{
				Name:     t.Name,
				Pattern:  t.String(),
				Length:   len(t.Pattern),
				Priority: t.Priority,
			}
			switch reason := bid(t, sp, i); reason {
			case "":
				a.Matched = true
				winners = append(winners, t)
			default:
				a.Reason = reason
			}
			step.Attempts = append(step.Attempts, a)
		}
		if len(winners) == 0 {
			// Record the failing position rather than dropping it.
			// This is exactly where the bids matter: "no lowering
			// exists" is only useful alongside which tile came closest
			// and what stopped it.
			step.Covered = sp.Slice(i, sp.Len()).String()
			ex.Steps = append(ex.Steps, step)
			break
		}
		pick := best(winners)
		step.Chosen = pick.Name
		step.Covered = sp.Slice(i, i+len(pick.Pattern)).String()
		step.Because = why(pick, winners)
		ex.Steps = append(ex.Steps, step)
		i += len(pick.Pattern)
	}

	ex.Leftover = sp.Slice(i, sp.Len())
	switch {
	case i == 0:
		ex.Note = fmt.Sprintf("no tile matched at the outermost layer, so %s has no lowering on %s",
			sp, r.Target)
	case ex.Leftover.Len() > 0:
		ex.Note = "what no tile claimed is traffic through the device, not configuration of it"
	}
	return ex
}

// bid returns "" when the tile matches, or the reason it does not.
func bid(t Tile, sp spine.Spine, i int) string {
	if i+len(t.Pattern) > sp.Len() {
		return fmt.Sprintf("needs %d layers, only %d remain", len(t.Pattern), sp.Len()-i)
	}
	for k, want := range t.Pattern {
		if got := sp.At(i + k).Name; got != want {
			return fmt.Sprintf("layer %d is %s, wants %s", i+k, got, want)
		}
	}
	if t.Guard != nil && !t.Guard(sp.Slice(i, i+len(t.Pattern))) {
		return "shape matches, but a field value ruled it out"
	}
	return ""
}

func why(pick Tile, all []Tile) string {
	if len(all) == 1 {
		return "the only tile that matched"
	}
	longest := 0
	for _, t := range all {
		if len(t.Pattern) > longest {
			longest = len(t.Pattern)
		}
	}
	tied := 0
	for _, t := range all {
		if len(t.Pattern) == longest {
			tied++
		}
	}
	if tied > 1 {
		return fmt.Sprintf("%d tiles matched %d layers; the lowest priority wins", tied, longest)
	}
	return fmt.Sprintf("longest match: %d layers against %d others", longest, len(all)-1)
}

func (e Explanation) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "spine:  %s\ntarget: %s\n", e.Spine, e.Target)
	for _, s := range e.Steps {
		if s.Chosen == "" {
			fmt.Fprintf(&b, "\nat layer %d: %s — nothing matched\n", s.At, s.Covered)
		} else {
			fmt.Fprintf(&b, "\nat layer %d: %s\n", s.At, s.Covered)
		}
		for _, a := range s.Attempts {
			switch {
			case a.Name == s.Chosen:
				fmt.Fprintf(&b, "  %-10s %-34s chosen — %s\n", a.Name, a.Pattern, s.Because)
			case a.Matched:
				fmt.Fprintf(&b, "  %-10s %-34s matched %d layers\n", a.Name, a.Pattern, a.Length)
			default:
				fmt.Fprintf(&b, "  %-10s %-34s %s\n", a.Name, a.Pattern, a.Reason)
			}
		}
	}
	if e.Leftover.Len() > 0 {
		fmt.Fprintf(&b, "\nleft over: %s\n", e.Leftover)
	}
	if e.Note != "" {
		fmt.Fprintf(&b, "\n%s\n", e.Note)
	}
	return b.String()
}
