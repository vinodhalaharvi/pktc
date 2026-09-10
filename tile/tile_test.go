package tile

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/pktc/command"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/spine"
)

func stub(name string, pattern ...string) Tile {
	return Tile{
		Name:    name,
		Pattern: pattern,
		Lower: func(spine.Spine, *env.Env) (command.Script, error) {
			return command.Script{command.New("", name)}, nil
		},
	}
}

func TestRegistryRejectsAmbiguity(t *testing.T) {
	_, err := NewRegistry("test",
		stub("A", "ipv4", "gre"),
		stub("B", "ipv4", "gre"),
	)
	if err == nil {
		t.Fatal("identical pattern and priority must be a load-time error")
	}
	if !strings.Contains(err.Error(), "distinct priority") {
		t.Errorf("error should say how to fix it, got %q", err)
	}
}

func TestRegistryAllowsPriorityBreak(t *testing.T) {
	a := stub("A", "ipv4", "gre")
	b := stub("B", "ipv4", "gre")
	b.Priority = 1
	if _, err := NewRegistry("test", a, b); err != nil {
		t.Fatalf("distinct priorities should load: %v", err)
	}
}

func TestRegistryRejectsMalformed(t *testing.T) {
	if _, err := NewRegistry("test", Tile{Name: "empty"}); err == nil {
		t.Error("an empty pattern must be rejected")
	}
	if _, err := NewRegistry("test", Tile{Name: "nolower", Pattern: []string{"ipv4"}}); err == nil {
		t.Error("a tile with no Lower must be rejected")
	}
}

func TestBestPrefersLongest(t *testing.T) {
	short := stub("short", "ipv4")
	long := stub("long", "ipv4", "gre")
	if got := best([]Tile{short, long}); got.Name != "long" {
		t.Errorf("got %s, want the longer match", got.Name)
	}
	if got := best([]Tile{long, short}); got.Name != "long" {
		t.Errorf("order must not matter, got %s", got.Name)
	}
}

func TestBestBreaksTiesOnPriority(t *testing.T) {
	lo := stub("lo", "ipv4", "gre")
	hi := stub("hi", "ipv4", "gre")
	hi.Priority = 5
	if got := best([]Tile{hi, lo}); got.Name != "lo" {
		t.Errorf("got %s, want the lower priority number", got.Name)
	}
}
