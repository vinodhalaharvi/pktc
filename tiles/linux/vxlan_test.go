package linux

import (
	"strings"
	"testing"
)

// The four VXLAN modes differ only in which fields are concrete and
// which are '*'. Each produces a different kernel device.
func TestVXLANModes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "point to point",
			src: `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
				(udp :dst-port 4789 (vxlan :vni 100 (ethernet (ipv4 :src "10.100.0.1/24"))))))`,
			want: `ip link add name vxlan100 type vxlan id 100 local 192.168.1.10 remote 192.168.1.20 dstport 4789 dev eth0
ip addr add 10.100.0.1/24 dev vxlan100
ip link set vxlan100 up`,
		},
		{
			// Same field, different kernel parameter: a multicast outer
			// destination is the group every peer joins, not a peer.
			name: "multicast group",
			src: `(configure (ipv4 :src "192.168.1.10" :dst "239.1.1.1"
				(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`,
			want: `ip link add name vxlan100 type vxlan id 100 local 192.168.1.10 group 239.1.1.1 dstport 4789 dev eth0
ip link set vxlan100 up`,
		},
		{
			name: "learned peers",
			src: `(configure (ipv4 :src "192.168.1.10" :dst *
				(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`,
			want: `ip link add name vxlan100 type vxlan id 100 local 192.168.1.10 dstport 4789 dev eth0
ip link set vxlan100 up`,
		},
		{
			// :vni * is collect-metadata mode, which has no VNI to be
			// named after, so the device gets an allocated name.
			name: "every vni",
			src: `(configure (ipv4 :src "192.168.1.10" :dst *
				(udp :dst-port 4789 (vxlan :vni * (ethernet)))))`,
			want: `ip link add name vxlan1 type vxlan external local 192.168.1.10 dstport 4789 dev eth0
ip link set vxlan1 up`,
		},
		{
			name: "source chosen by the underlay",
			src: `(configure (ipv4 :src * :dst "192.168.1.20"
				(udp :dst-port 4789 (vxlan :vni 7 (ethernet)))))`,
			want: `ip link add name vxlan7 type vxlan id 7 remote 192.168.1.20 dstport 4789 dev eth0
ip link set vxlan7 up`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, applied := lower(t, tt.src)
			if got != tt.want {
				t.Errorf("script mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tt.want)
			}
			if len(applied) != 1 || applied[0] != "VXLAN" {
				t.Errorf("tiles = %v, want [VXLAN]", applied)
			}
		})
	}
}

func TestVXLANRefusals(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// The kernel default is 8472 and the IANA value is 4789.
			// Guessing either one produces a device that comes up
			// cleanly and carries nothing.
			"port omitted",
			`(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (udp (vxlan :vni 100 (ethernet)))))`,
			"kernel default is 8472",
		},
		{
			"port dynamic",
			`(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (udp :dst-port * (vxlan :vni 100 (ethernet)))))`,
			"listens on one port",
		},
		{
			"vni omitted",
			`(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (udp :dst-port 4789 (vxlan (ethernet)))))`,
			"missing required property :vni",
		},
		{
			"destination omitted",
			`(configure (ipv4 :src "192.168.1.10" (udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`,
			"missing required property :dst",
		},
		{
			"source omitted",
			`(configure (ipv4 :dst "192.168.1.20" (udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`,
			"missing required property :src",
		},
		{
			"vni out of range",
			`(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20" (udp :dst-port 4789 (vxlan :vni 16777216 (ethernet)))))`,
			"24-bit VNI range",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := lowerErr(t, tt.src)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// Longest match must prefer VXLAN over any shorter tile that also
// begins at ipv4.
func TestVXLANWinsLongestMatch(t *testing.T) {
	_, applied := lower(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(udp :dst-port 4789 (vxlan :vni 100 (ethernet (ipv4 :src "10.0.0.1/24" (tcp :dst-port 80)))))))`)
	if len(applied) != 1 || applied[0] != "VXLAN" {
		t.Errorf("tiles = %v, want [VXLAN]", applied)
	}
}
