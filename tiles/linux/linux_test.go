package linux

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/pktc/ast"
	"github.com/vinodhalaharvi/pktc/env"
	"github.com/vinodhalaharvi/pktc/sexp"
	"github.com/vinodhalaharvi/pktc/spine"
)

func lower(t *testing.T, src string) (string, []string) {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	root, err := ast.Build(forms[0])
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	sp, err := spine.FromForm(root)
	if err != nil {
		t.Fatalf("spine: %v", err)
	}
	reg, err := Registry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	res, err := reg.Cover(sp, env.New("eth0"))
	if err != nil {
		t.Fatalf("cover: %v", err)
	}
	return res.Script.Plain(), res.Applied
}

func lowerErr(t *testing.T, src string) error {
	t.Helper()
	forms, err := sexp.Read("t.lisp", src)
	if err != nil {
		return err
	}
	root, err := ast.Build(forms[0])
	if err != nil {
		return err
	}
	sp, err := spine.FromForm(root)
	if err != nil {
		return err
	}
	reg, err := Registry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	_, err = reg.Cover(sp, env.New("eth0"))
	return err
}

func TestGolden(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		tiles string
		want  string
	}{
		{
			name:  "gre",
			src:   `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre (ipv4 :src "10.20.0.1/30"))))`,
			tiles: "GRE",
			want: `ip link add name gre1 type gre local 192.168.1.10 remote 192.168.1.20
ip addr add 10.20.0.1/30 dev gre1
ip link set gre1 up`,
		},
		{
			name:  "gre with key",
			src:   `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre :key 1001 (ipv4 :src "10.20.0.1/30"))))`,
			tiles: "GRE",
			want: `ip link add name gre1 type gre local 192.168.1.10 remote 192.168.1.20 key 1001
ip addr add 10.20.0.1/30 dev gre1
ip link set gre1 up`,
		},
		{
			// Same first two layers as gre; the inner layer selects the device.
			name:  "gretap",
			src:   `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre :key 500 (ethernet (ipv4 :src "10.50.0.1/24")))))`,
			tiles: "GRETAP",
			want: `ip link add name gretap1 type gretap local 192.168.1.10 remote 192.168.1.20 key 500
ip addr add 10.50.0.1/24 dev gretap1
ip link set gretap1 up`,
		},
		{
			// tcp below the tunnel is traffic; longest match leaves it alone.
			name:  "ipip with payload",
			src:   `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (ipv4 :src "10.30.0.1/30" (tcp :dst-port 22))))`,
			tiles: "IPIP",
			want: `ip link add name ipip1 type ipip local 192.168.1.10 remote 192.168.1.20
ip addr add 10.30.0.1/30 dev ipip1
ip link set ipip1 up`,
		},
		{
			name:  "gretap without an inner address",
			src:   `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre (ethernet))))`,
			tiles: "GRETAP",
			want: `ip link add name gretap1 type gretap local 192.168.1.10 remote 192.168.1.20
ip link set gretap1 up`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, applied := lower(t, tt.src)
			if got != tt.want {
				t.Errorf("script mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tt.want)
			}
			if strings.Join(applied, " → ") != tt.tiles {
				t.Errorf("tiles = %v, want %s", applied, tt.tiles)
			}
		})
	}
}

func TestLoweringErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			"no tile",
			`(configure (ethernet (mpls (ipv4 :src "10.0.0.1/24"))))`,
			"no lowering exists",
		},
		{
			"missing remote",
			`(configure (ipv4 :src "192.168.1.10" (gre (ipv4 :src "10.20.0.1/30"))))`,
			"missing required property :dst",
		},
		{
			"dynamic remote on a point-to-point tunnel",
			`(configure (ipv4 :src "192.168.1.10" :dst * (gre (ipv4 :src "10.20.0.1/30"))))`,
			"has no lowering for a point-to-point tunnel",
		},
		{
			"bare inner address",
			`(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (gre (ipv4 :src "10.20.0.1"))))`,
			"needs a prefix length",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := lowerErr(t, tt.src)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestRegistryValidates(t *testing.T) {
	if _, err := Registry(); err != nil {
		t.Fatalf("the shipped tile set must load: %v", err)
	}
}
