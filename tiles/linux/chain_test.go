package linux

import (
	"strings"
	"testing"
)

const nested = `(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (udp :dst-port 4789
      (vxlan :vni 100
        (ethernet
          (ipv4 :src "10.100.0.1/24" :dst "10.100.0.2"
            (udp :dst-port 4789
              (vxlan :vni 200
                (ethernet
                  (ipv4 :src "10.200.0.1/24"))))))))))`

// A tunnel inside a tunnel is two tiles covering one spine. There is no
// combined tile, and neither tile knows the other exists: the outer
// one's device simply becomes the inner one's underlay.
func TestChainedTiles(t *testing.T) {
	got, applied := lower(t, nested)
	want := `ip link add name vxlan100 type vxlan id 100 local 192.168.1.10 remote 192.168.1.20 dstport 4789 dev eth0
ip addr add 10.100.0.1/24 dev vxlan100
ip link add name vxlan200 type vxlan id 200 local 10.100.0.1 remote 10.100.0.2 dstport 4789 dev vxlan100
ip addr add 10.200.0.1/24 dev vxlan200
ip link set vxlan100 up
ip link set vxlan200 up`
	if got != want {
		t.Errorf("script mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if strings.Join(applied, " ") != "VXLAN VXLAN" {
		t.Errorf("tiles = %v, want two VXLAN tiles", applied)
	}
}

// The dependency edge between the tiles: an inner tunnel must be built
// on the device the outer tile created, not on the physical underlay.
func TestInnerTileAttachesToOuterDevice(t *testing.T) {
	got, _ := lower(t, nested)
	lines := strings.Split(got, "\n")
	if !strings.HasSuffix(lines[2], "dev vxlan100") {
		t.Errorf("inner tunnel should attach to vxlan100, got:\n%s", lines[2])
	}
	if !strings.HasSuffix(lines[0], "dev eth0") {
		t.Errorf("outer tunnel should attach to the underlay, got:\n%s", lines[0])
	}
}

// A prefix on an intermediate layer is the address the outer device
// needs before anything can stand on it. It has to be added before the
// inner device is created, and the bare address is reused as that
// tunnel's local endpoint.
func TestIntermediatePrefixBecomesUnderlayAddress(t *testing.T) {
	got, _ := lower(t, nested)
	lines := strings.Split(got, "\n")
	if lines[1] != "ip addr add 10.100.0.1/24 dev vxlan100" {
		t.Fatalf("expected the middle address on vxlan100, got:\n%s", lines[1])
	}
	if !strings.Contains(lines[2], "local 10.100.0.1 ") {
		t.Errorf("the bare form of that address should be the inner tunnel's local endpoint:\n%s", lines[2])
	}
}

// Every device in the chain has to come up, outermost first: an inner
// tunnel cannot carry traffic over an underlay that is down.
func TestAllDevicesBroughtUp(t *testing.T) {
	got, _ := lower(t, nested)
	outer := strings.Index(got, "ip link set vxlan100 up")
	inner := strings.Index(got, "ip link set vxlan200 up")
	if outer < 0 || inner < 0 {
		t.Fatalf("both devices must be brought up:\n%s", got)
	}
	if outer > inner {
		t.Error("the outer device must come up before the inner one")
	}
}

// A bare address names an endpoint without claiming the device below
// already has it, so nothing is emitted for the physical underlay.
func TestBareOuterAddressAddsNothing(t *testing.T) {
	got, _ := lower(t, `(configure (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
		(udp :dst-port 4789 (vxlan :vni 100 (ethernet)))))`)
	if strings.Contains(got, "dev eth0") && strings.Contains(got, "ip addr add 192.168.1.10") {
		t.Errorf("a bare outer address must not be configured on the underlay:\n%s", got)
	}
}
