# pktc

[![ci](https://github.com/vinodhalaharvi/pktc/actions/workflows/ci.yml/badge.svg)](https://github.com/vinodhalaharvi/pktc/actions/workflows/ci.yml)

Describe the packet you expect on the wire. pktc works out which kernel
primitive produces it.

```lisp
(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (udp :dst-port 4789
      (vxlan :vni 100
        (ethernet
          (ipv4 :src "10.100.0.1/24"))))))
```

```
$ pktc lower testdata/vxlan.lisp
ip link add name vxlan100 type vxlan id 100 local 192.168.1.10 remote 192.168.1.20 dstport 4789 dev eth0
ip addr add 10.100.0.1/24 dev vxlan100
ip link set vxlan100 up
```

Nothing in that file named a device type. `netplan`, `systemd-networkd`,
`nmstate` and NetworkManager all make you name the mechanism first and
then fill in its parameters. This goes the other way.

**pktc prints configuration. It never applies it.**

## The inner layer picks the device

Two files, identical for their first two layers:

```lisp
(ipv4 :src "..." :dst "..." (gre (ipv4 :src "10.20.0.1/30")))          ; L3 over L3
(ipv4 :src "..." :dst "..." (gre (ethernet (ipv4 :src "10.50.0.1/24")))) ; L2 over L3
```

```
ip link add name gre1 type gre local ... remote ...
ip link add name gretap1 type gretap local ... remote ...
```

## How it works

An encapsulation stack does not branch: each layer carries exactly one
thing. So the packet tree flattens to a **spine**, and covering it with
device shapes is *instruction selection by tree tiling* — the technique
a compiler back end uses to pick machine instructions, except the tiles
are Linux tunnel devices and the emitted code is `iproute2` commands.

Because the spine is a list rather than a tree, greedy longest-match is
optimal and the tiler is under fifty lines.

```
ipv4 · udp · vxlan · ethernet · ipv4
└─────── VXLAN tile ────────┘   └─ address on the device
```

Whatever the tiles do not claim is traffic, not configuration:

```
$ pktc lower testdata/ipip.lisp
ip link add name ipip1 type ipip local 192.168.1.10 remote 192.168.1.20
ip addr add 10.30.0.1/30 dev ipip1
ip link set ipip1 up

# tcp below the tunnel is traffic, not configuration
```

Longest match drew that line without being told where it is.

## Nested tunnels need no special case

A tunnel inside a tunnel is two tiles covering one spine. There is no
combined tile; the outer tile's device becomes the inner tile's
underlay.

```
$ pktc lower testdata/vxlan-nested.lisp
# tiles: VXLAN → VXLAN

ip link add name vxlan100 type vxlan id 100 local 192.168.1.10 remote 192.168.1.20 dstport 4789 dev eth0
ip addr add 10.100.0.1/24 dev vxlan100
ip link add name vxlan200 type vxlan id 200 local 10.100.0.1 remote 10.100.0.2 dstport 4789 dev vxlan100
ip addr add 10.200.0.1/24 dev vxlan200
ip link set vxlan100 up
ip link set vxlan200 up
```

## One packet, many packets

A packet describes a single frame between two endpoints. A device serves
a whole class of frames. Where the two disagree, `*` says which field
varies — and each choice selects a different kernel mode rather than
relaxing a constraint.

| written | emitted |
|---|---|
| `:dst "192.168.1.20"` | `remote 192.168.1.20` |
| `:dst "239.1.1.1"` | `group 239.1.1.1` |
| `:dst *` | neither: peers are learned |
| `:vni *` | `external` (collect-metadata) |

Absent is a third state, and it is an error:

```
$ pktc lower no-port.lisp
pktc: no-port.lisp:1:58: (udp ...): missing required property :dst-port;
      pktc will not guess, because the kernel default is 8472 and the
      IANA value is 4789
```

That default is real, and a device built on the wrong port comes up
cleanly and carries nothing.

## Both ends from one description

A tunnel is symmetric, so the far end is the near end with its endpoints
exchanged. What cannot be derived is reported rather than invented.

```
$ pktc mirror -peer-inner 10.100.0.2/24 -peer-underlay wan testdata/vxlan.lisp

# derived: :src and :dst exchanged on (ipv4 ...)
# derived: :vni on (vxlan ...) is the same at both ends
# derived: inner address on (ipv4 ...) set to 10.100.0.2/24

ip link add name vxlan100 type vxlan id 100 local 192.168.1.20 remote 192.168.1.10 dstport 4789 dev wan
ip addr add 10.100.0.2/24 dev vxlan100
```

Without `-peer-inner` it drops the address and says so, rather than
incrementing the host part and being right often enough to be trusted.

The star mirrors along with its value: `:src X :dst *` becomes
`:src * :dst X`. The end that learned its peers becomes the end that is
learned.

## What the wire should carry

The same tree read a third way:

```
$ pktc expect testdata/vxlan.lisp
filter: udp port 4789

expect on the underlay:
  outer source          192.168.1.10
  UDP destination port  4789
  outer destination     192.168.1.20
  encapsulation         VXLAN
  VXLAN VNI             100
```

Which closes a loop nothing else in the design closes. `make netns`
builds two namespaces on a veth pair, configures the near end from
`lower` and the far end from `mirror`, pushes traffic through the
tunnel, and captures the underlay:

```
IP 192.168.1.10.34533 > 192.168.1.20.4789: VXLAN, flags [I] (0x08), vni 100
```

If the capture and the prediction disagree, one of those readings of the
tree is wrong. A golden test proves the intended commands were emitted;
a kernel test proves the device was created. Only this says the device
does what the tree claimed it would.

## Install

```
go install github.com/vinodhalaharvi/pktc/cmd/pktc@latest
```

Or from a checkout:

```
make check      # gofmt, vet, unit tests
make demo       # every fixture, one line each
make netns      # run the generated configuration against a real kernel
```

The kernel tests need Linux and root, and skip rather than fail when the
machine cannot support them.

## Commands

```
pktc parse <file>            print the encapsulation spine
pktc lower [flags] <file>    print the commands that produce it
pktc mirror [flags] <file>   print both ends of the tunnel
pktc expect <file>           print what the underlay should carry
pktc explain <file>          print which tiles bid and why one won
```

## Why that tile

Instruction selection is easy to trust when it works and opaque when it
does not, so the decision is inspectable:

```
$ pktc explain testdata/vlan-vxlan.lisp
spine:  ethernet · vlan · ipv4 · udp · vxlan · ethernet · ipv4

at layer 0: ethernet · vlan
  VLAN       ethernet · vlan                chosen — the only tile that matched
  VXLAN      ipv4 · udp · vxlan · ethernet  layer 0 is ethernet, wants ipv4
  ...

at layer 2: ipv4 · udp · vxlan · ethernet
  VXLAN      ipv4 · udp · vxlan · ethernet  chosen — the only tile that matched
  WireGuard  ipv4 · udp · wireguard · ipv4  layer 4 is vxlan, wants wireguard
  GRE        ipv4 · gre · ipv4              layer 3 is udp, wants gre
```

Naming the layer that stopped a tile is the difference between a
near-miss and a no-match: WireGuard got three layers in, GRE got one.
Failed positions are reported too, so `no lowering exists` arrives with
the bids that came closest.

## Tiles

| shape | device |
|---|---|
| `ipv4 · ipv4` | `ipip` |
| `ipv4 · gre · ipv4` | `gre` |
| `ipv4 · gre · ethernet` | `gretap` |
| `ipv4 · udp · vxlan · ethernet` | `vxlan` |
| `ethernet · vlan` | `vlan` |
| `ipv4 · udp · wireguard · ipv4` | `wireguard` |

Geneve and XFRM are still absent.

WireGuard is the one asymmetric tile. Key material is machine state and
must never appear in a tree, so the tree names a peer and the name
resolves to a file. It is also the only tile whose peer may be
unaddressable: `:dst *` means a peer behind NAT that announces itself on
first handshake.

```
$ pktc mirror -peer-inner 10.44.0.2/24 testdata/wireguard.lisp
# derived: :src and :dst exchanged on (ipv4 ...)
# NOT DERIVED: key material on (wireguard ...): each end holds its own
#              private key and needs the other's public key, which no
#              reading of this tree can supply
```

That is the first asymmetry that is structural rather than an address,
and reporting it beats emitting a script that looks complete.

## Status

Early, and honest about it. The design is written up in
[docs/DESIGN.md](docs/DESIGN.md), including an appendix recording what
building it confirmed, changed and deferred.

A file may hold several rules; they share one namespace of interface
names, so the second GRE tunnel is `gre2`.

Known gaps: six tiles, one backend. VLAN and WireGuard were written where their kernel modules were
unavailable, so CI is their first real check rather than a local run.

## License

MIT
