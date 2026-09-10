package linux

import (
	"strings"
	"testing"
)

func TestVLAN(t *testing.T) {
	got, applied := lower(t, `(configure (ethernet (vlan :id 100 (ipv4 :src "10.0.0.1/24"))))`)
	want := `ip link add link eth0 name eth0.100 type vlan id 100
ip addr add 10.0.0.1/24 dev eth0.100
ip link set eth0.100 up`
	if got != want {
		t.Errorf("script mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if len(applied) != 1 || applied[0] != "VLAN" {
		t.Errorf("tiles = %v", applied)
	}
}

// A tagged interface is usually the first of a chain: something else is
// built on it. This is the realistic version of the nested case.
func TestVLANCarriesVXLAN(t *testing.T) {
	got, applied := lower(t, `(configure (ethernet (vlan :id 100
		(ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		  (udp :dst-port 4789 (vxlan :vni 200 (ethernet (ipv4 :src "10.200.0.1/24"))))))))`)
	want := `ip link add link eth0 name eth0.100 type vlan id 100
ip link add name vxlan200 type vxlan id 200 local 192.168.1.10 remote 192.168.1.20 dstport 4789 dev eth0.100
ip addr add 10.200.0.1/24 dev vxlan200
ip link set eth0.100 up
ip link set vxlan200 up`
	if got != want {
		t.Errorf("script mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if strings.Join(applied, " → ") != "VLAN → VXLAN" {
		t.Errorf("tiles = %v", applied)
	}
}

func TestVLANRefusals(t *testing.T) {
	for _, tt := range []struct{ name, src, want string }{
		{"dynamic tag", `(configure (ethernet (vlan :id * (ipv4 :src "10.0.0.1/24"))))`, "carries one tag"},
		{"reserved tag", `(configure (ethernet (vlan :id 4095 (ipv4 :src "10.0.0.1/24"))))`, "usable VLAN range"},
		{"zero tag", `(configure (ethernet (vlan :id 0 (ipv4 :src "10.0.0.1/24"))))`, "usable VLAN range"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := lowerErr(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestWireGuard(t *testing.T) {
	got, _ := lower(t, `(configure (ipv4 :src "198.51.100.10" :dst "198.51.100.20"
		(udp :src-port 51820 :dst-port 51820 (wireguard :peer "peer-b" (ipv4 :src "10.44.0.1/24")))))`)
	want := `ip link add name wg1 type wireguard
wg set wg1 listen-port 51820 private-key /etc/wireguard/wg1.key
wg set wg1 peer "$(cat /etc/wireguard/peer-b.pub)" endpoint 198.51.100.20:51820 allowed-ips 10.44.0.0/24
ip addr add 10.44.0.1/24 dev wg1
ip link set wg1 up`
	if got != want {
		t.Errorf("script mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// allowed-ips is the network the inner prefix sits in, not the host
// address written down.
func TestAllowedIPsIsTheNetwork(t *testing.T) {
	got, _ := lower(t, `(configure (ipv4 :src "198.51.100.10" :dst "198.51.100.20"
		(udp :src-port 51820 :dst-port 51820 (wireguard :peer "b" (ipv4 :src "10.44.3.77/16")))))`)
	if !strings.Contains(got, "allowed-ips 10.44.0.0/16") {
		t.Errorf("expected the masked network, got:\n%s", got)
	}
}

// WireGuard is the only tile whose peer may be unaddressable: a peer
// behind NAT announces itself on first handshake.
func TestWireGuardDynamicEndpoint(t *testing.T) {
	got, _ := lower(t, `(configure (ipv4 :src "198.51.100.10" :dst *
		(udp :src-port 51820 :dst-port 51820 (wireguard :peer "roamer" (ipv4 :src "10.44.0.1/24")))))`)
	if strings.Contains(got, "endpoint") {
		t.Errorf("a dynamic destination must not produce an endpoint:\n%s", got)
	}
	if !strings.Contains(got, "allowed-ips 10.44.0.0/24") {
		t.Errorf("allowed-ips is still required:\n%s", got)
	}
}

// A tree is something people paste into issues. Keys must be named,
// never written.
func TestWireGuardRefusals(t *testing.T) {
	for _, tt := range []struct{ name, src, want string }{
		{"no peer", `(configure (ipv4 :src "1.1.1.1" :dst "2.2.2.2" (udp :src-port 51820 (wireguard (ipv4 :src "10.44.0.1/24")))))`, "Keys are not packet structure"},
		{"no listen port", `(configure (ipv4 :src "1.1.1.1" :dst "2.2.2.2" (udp (wireguard :peer "b" (ipv4 :src "10.44.0.1/24")))))`, "listens on a fixed port"},
		{"no inner prefix", `(configure (ipv4 :src "1.1.1.1" :dst "2.2.2.2" (udp :src-port 51820 (wireguard :peer "b" (ipv4)))))`, "allowed-ips is derived from"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := lowerErr(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}
