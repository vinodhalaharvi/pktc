package wire

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
)

func expect(t *testing.T, src string) Expectation {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		t.Fatal(err)
	}
	root, err := ast.Build(forms[0])
	if err != nil {
		t.Fatal(err)
	}
	sp, err := spine.FromForm(root)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Expect(sp)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func has(e Expectation, label string) (Fact, bool) {
	for _, f := range e.Facts {
		if f.Label == label {
			return f, true
		}
	}
	return Fact{}, false
}

func TestFilters(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"vxlan", `(configure (ipv4 :src "1.1.1.1" :dst "2.2.2.2" (udp :dst-port 4789 (vxlan :vni 5 (ethernet)))))`, "udp port 4789"},
		{"gre", `(configure (ipv4 :src "1.1.1.1" :dst "2.2.2.2" (gre (ipv4 :src "10.0.0.1/30"))))`, "proto gre"},
		{"ipip", `(configure (ipv4 :src "1.1.1.1" :dst "2.2.2.2" (ipv4 :src "10.0.0.1/30")))`, "proto 4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expect(t, tt.src).Filter; got != tt.want {
				t.Errorf("filter = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVXLANFacts(t *testing.T) {
	e := expect(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`)
	for label, want := range map[string]string{
		"outer source":         "192.168.1.10",
		"outer destination":    "192.168.1.20",
		"UDP destination port": "4789",
		"VXLAN VNI":            "100",
	} {
		f, ok := has(e, label)
		if !ok {
			t.Errorf("no fact for %q", label)
			continue
		}
		if f.Value != want {
			t.Errorf("%s = %q, want %q", label, f.Value, want)
		}
	}
}

// A '*' says the field varies across the packets this device carries,
// so there is nothing to assert about any one of them.
func TestDynamicFieldsArePredictedNothing(t *testing.T) {
	e := expect(t, `(configure (ipv4 :src "192.168.1.10" :dst *
		(udp :dst-port 4789 (vxlan :vni * (ethernet)))))`)
	if _, ok := has(e, "VXLAN VNI"); ok {
		t.Error("a dynamic VNI must not be predicted")
	}
	if _, ok := has(e, "outer destination"); ok {
		t.Error("a dynamic destination must not be predicted")
	}
	if _, ok := has(e, "outer source"); !ok {
		t.Error("the concrete source should still be predicted")
	}
}

// The prediction is checked against real capture text, so the matching
// has to survive tcpdump's actual formatting.
func TestCheckAgainstRealCaptureText(t *testing.T) {
	e := expect(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`)
	captured := "02:29:24.663079 IP 192.168.1.10.34533 > 192.168.1.20.4789: VXLAN, flags [I] (0x08), vni 100"
	if missing := e.Check(captured); len(missing) != 0 {
		t.Errorf("these facts were not found in a genuine capture line: %v", missing)
	}
}

func TestCheckReportsDisagreement(t *testing.T) {
	e := expect(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`)
	wrongVNI := "IP 192.168.1.10.34533 > 192.168.1.20.4789: VXLAN, flags [I] (0x08), vni 999"
	missing := e.Check(wrongVNI)
	if len(missing) != 1 || missing[0].Label != "VXLAN VNI" {
		t.Errorf("expected exactly the VNI to be reported missing, got %v", missing)
	}
}

func TestUnpredictable(t *testing.T) {
	forms, _ := sexp.Read("t.lisp", `(configure (ethernet (ipv4 :src "10.0.0.1/24")))`)
	root, _ := ast.Build(forms[0])
	sp, _ := spine.FromForm(root)
	if _, err := Expect(sp); err == nil || !strings.Contains(err.Error(), "only IP underlays") {
		t.Errorf("a non-IP outer layer should be refused, got %v", err)
	}
}
