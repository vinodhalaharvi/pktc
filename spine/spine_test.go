package spine

import (
	"testing"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/sexp"
)

func form(t *testing.T, src string) *ast.Node {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	n, err := ast.Build(forms[0])
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return n
}

func TestExtract(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			"vxlan",
			`(configure (ipv4 (udp :dst-port 4789 (vxlan :vni 100 (ethernet (ipv4 :src "10.0.0.1/24"))))))`,
			"ipv4 · udp · vxlan · ethernet · ipv4",
		},
		{
			"gre",
			`(configure (ipv4 (gre (ipv4 :src "10.0.0.1/30"))))`,
			"ipv4 · gre · ipv4",
		},
		{
			"payload below the tunnel",
			`(configure (ipv4 (ipv4 (tcp :dst-port 22))))`,
			"ipv4 · ipv4 · tcp",
		},
		{
			"single layer",
			`(configure (ipv4))`,
			"ipv4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp, err := FromForm(form(t, tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if got := sp.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFromFormErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"wrong form", `(send (ipv4))`},
		{"no packet", `(configure)`},
		{"two packets", `(configure (ipv4) (ipv6))`},
		{"properties on configure", `(configure :name "x" (ipv4))`},
		{"branching stack", `(configure (ipv4 (udp) (tcp)))`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := FromForm(form(t, tt.src)); err == nil {
				t.Fatalf("expected an error for %q", tt.src)
			}
		})
	}
}

func TestSlice(t *testing.T) {
	sp, err := FromForm(form(t, `(configure (ipv4 (udp (vxlan (ethernet)))))`))
	if err != nil {
		t.Fatal(err)
	}
	if got := sp.Slice(1, 3).String(); got != "udp · vxlan" {
		t.Errorf("got %q", got)
	}
}
