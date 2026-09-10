package tile

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
)

func sp(t *testing.T, src string) spine.Spine {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		t.Fatal(err)
	}
	root, err := ast.Build(forms[0])
	if err != nil {
		t.Fatal(err)
	}
	s, err := spine.FromForm(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func reg(t *testing.T, tiles ...Tile) *Registry {
	t.Helper()
	r, err := NewRegistry("test", tiles...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func find(e Explanation, step int, name string) (Attempt, bool) {
	for _, a := range e.Steps[step].Attempts {
		if a.Name == name {
			return a, true
		}
	}
	return Attempt{}, false
}

// The reason a tile lost is the whole point: "layer 3 is udp, wants
// gre" tells you where to look, "no match" does not.
func TestExplainNamesTheLayerThatStoppedIt(t *testing.T) {
	r := reg(t,
		stub("SHORT", "ipv4", "ipv4"),
		stub("LONG", "ipv4", "udp", "vxlan", "ethernet"),
	)
	e := r.Explain(sp(t, `(configure (ipv4 (udp (vxlan (ethernet (ipv4))))))`))
	a, ok := find(e, 0, "SHORT")
	if !ok {
		t.Fatal("no attempt recorded for SHORT")
	}
	if !strings.Contains(a.Reason, "layer 1 is udp") {
		t.Errorf("reason = %q, want it to name the offending layer", a.Reason)
	}
}

func TestExplainReportsTooFewLayers(t *testing.T) {
	r := reg(t, stub("LONG", "ipv4", "udp", "vxlan", "ethernet"))
	e := r.Explain(sp(t, `(configure (ipv4 (udp)))`))
	if !strings.Contains(e.Note, "no lowering") {
		t.Errorf("note = %q", e.Note)
	}
	// The failing position is still reported, with the reason.
	a, ok := find(e, 0, "LONG")
	if !ok {
		t.Fatal("the bid at the failing position must still be recorded")
	}
	if !strings.Contains(a.Reason, "only 2 remain") {
		t.Errorf("reason = %q, want it to say how many layers were left", a.Reason)
	}
}

func TestExplainRecordsWhyTheWinnerWon(t *testing.T) {
	short := stub("SHORT", "ipv4", "ipv4")
	long := stub("LONG", "ipv4", "ipv4", "tcp")
	e := reg(t, short, long).Explain(sp(t, `(configure (ipv4 (ipv4 (tcp))))`))
	if e.Steps[0].Chosen != "LONG" {
		t.Fatalf("chose %q, want LONG", e.Steps[0].Chosen)
	}
	if !strings.Contains(e.Steps[0].Because, "longest match") {
		t.Errorf("because = %q", e.Steps[0].Because)
	}
	if a, _ := find(e, 0, "SHORT"); !a.Matched {
		t.Error("SHORT matched too and should be recorded as a losing bid, not a non-match")
	}
}

func TestExplainRecordsPriorityTiebreak(t *testing.T) {
	lo := stub("LO", "ipv4", "ipv4")
	hi := stub("HI", "ipv4", "ipv4")
	hi.Priority = 5
	e := reg(t, hi, lo).Explain(sp(t, `(configure (ipv4 (ipv4)))`))
	if e.Steps[0].Chosen != "LO" {
		t.Fatalf("chose %q, want the lower priority number", e.Steps[0].Chosen)
	}
	if !strings.Contains(e.Steps[0].Because, "priority") {
		t.Errorf("because = %q, want it to mention the tiebreak", e.Steps[0].Because)
	}
}

func TestExplainRecordsAGuardRejection(t *testing.T) {
	guarded := stub("GUARDED", "ipv4", "ipv4")
	guarded.Guard = func(spine.Spine) bool { return false }
	e := reg(t, guarded).Explain(sp(t, `(configure (ipv4 (ipv4)))`))
	a, ok := find(e, 0, "GUARDED")
	if !ok {
		t.Fatal("no attempt recorded")
	}
	if a.Matched || !strings.Contains(a.Reason, "field value") {
		t.Errorf("a guard rejection should be distinguished from a shape mismatch, got %q", a.Reason)
	}
}

// Explain must walk the same algorithm Cover does, or it explains a
// decision that was never made.
func TestExplainAgreesWithCover(t *testing.T) {
	r := reg(t,
		stub("A", "ipv4", "udp", "vxlan", "ethernet"),
		stub("B", "ipv4", "ipv4"),
	)
	s := sp(t, `(configure (ipv4 (udp (vxlan (ethernet (ipv4 (ipv4)))))))`)
	res, err := r.Cover(s, env.New("eth0"))
	if err != nil {
		t.Fatal(err)
	}
	e := r.Explain(s)
	var chosen []string
	for _, st := range e.Steps {
		if st.Chosen != "" { // the trailing step records a failed position
			chosen = append(chosen, st.Chosen)
		}
	}
	if strings.Join(chosen, ",") != strings.Join(res.Applied, ",") {
		t.Errorf("Explain chose %v, Cover applied %v", chosen, res.Applied)
	}
	if e.Leftover.String() != res.Payload.String() {
		t.Errorf("Explain leftover %q, Cover payload %q", e.Leftover, res.Payload)
	}
}

var _ = command.Script{}
