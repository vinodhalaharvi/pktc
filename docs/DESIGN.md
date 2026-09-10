# From Packet Tree to Link Configuration

### A design report on compiling packet-shaped S-expressions into Linux tunnel devices

---

## 1. The idea in one sentence

Every existing Linux network configuration tool — `netplan`, `systemd-networkd`, `nmstate`, NetworkManager — makes you **name the mechanism** and then fill in its parameters. You say "I want a VXLAN device" and then supply an id, a local address, a remote address, a port.

This inverts that. You describe **the packet you expect to appear on the wire**, and the compiler derives which kernel primitive is required to produce it.

```lisp
(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (udp :dst-port 4789
      (vxlan :vni 100
        (ethernet
          (ipv4 :src "10.100.0.1/24"))))))
```

lowers to:

```bash
ip link add vxlan100 type vxlan \
    id 100 local 192.168.1.10 remote 192.168.1.20 \
    dstport 4789 dev eth0
ip addr add 10.100.0.1/24 dev vxlan100
ip link set vxlan100 up
```

The user never wrote the word `vxlan` as a *device type*. They wrote it as a *header that appears in the packet*. The device type was inferred.

---

## 2. Why this is a compiler problem, and which one

This is **instruction selection by tree tiling**, the standard back-end technique from compiler literature (BURS, maximal munch, iburg, twig).

The mapping is exact:

| Compiler concept | Here |
|---|---|
| Expression tree | Packet tree |
| Instruction / tile | A recognizable Linux tunnel shape |
| Tile pattern | A sequence of protocol nodes |
| Emitted machine code | `iproute2` commands |
| Register allocation | Interface naming |
| Calling convention | Facts the tree can't carry (see §5) |

Naming it correctly matters because it hands you the algorithms and the failure modes for free, rather than discovering them one at a time.

### The problem is easier here than in a real compiler

Instruction selection is hard because expression trees **branch**. `add(mul(a,b), c)` has two independent subtrees, and covering them optimally requires dynamic programming over a genuine tree.

An encapsulation stack does not branch. Each layer has exactly one child. **The tree is a spine.**

This is not a simplifying assumption. A header has *a* payload field, so
the single-child property follows from what encapsulation is, and no
encapsulating protocol is being excluded by it. Packets that branch —
SCTP chunks, DNS resource records, A-MSDU subframes, IPv6 option chains
— are containers of siblings rather than one-inside-another, and are a
different shape of problem.

The claim to state precisely is therefore bounded: *within one
encapsulation stack*, covering is list segmentation and greedy
longest-match is optimal. It is not a general result about the tool.
Aggregation devices (bridges, bonds, teams) are N-ary and would need a
real tree cover, at which point the optimality claim stops holding. They
are also not described by a packet, so the inversion this design rests
on does not reach them — which is a better reason to exclude them than
the algorithm being harder.

So the problem collapses from "optimal tree cover" to "optimal segmentation of a list," which greedy longest-match solves optimally, deterministically, in a single pass. No BURS. No dynamic programming.

```
tile(spine):
    i = 0
    while i < len(spine):
        candidates = [t for t in tiles if t.matches(spine, i)]
        if candidates is empty:
            error("no lowering exists for " + spine[i:])
        pick = max(candidates, key=(t.length, -t.priority))
        emit(pick.lower(spine[i : i+pick.length]))
        i += pick.length
```

That is the entire algorithm. Roughly forty lines with error handling.

---

## 3. The tile

```go
type Tile struct {
    Name     string
    Pattern  []NodeMatcher              // consecutive spine nodes
    Priority int                        // tiebreak only
    Guard    func(Spine) bool           // field-value conditions
    Lower    func(Spine, Env) ([]Command, error)
}
```

`Pattern` matches on **protocol names**. `Guard` checks **field values**. Splitting these two is what keeps the matcher tractable: `ipv4 · udp · vxlan · ethernet` is a pattern, `dport == 4789` is a guard.

An initial tile set of six or eight covers the useful ground:

```
VXLAN     ipv4 · udp · vxlan · ethernet
GENEVE    ipv4 · udp · geneve · ethernet
GRE       ipv4 · gre · ipv4
GRETAP    ipv4 · gre · ethernet
IPIP      ipv4 · ipv4
SIT       ipv4 · ipv6
WIREGUARD ipv4 · udp · wireguard · ipv4
XFRM      ipv4 · esp
```

---

## 4. Worked examples

### 4.1 The clean case

```lisp
(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (udp :dst-port 4789
      (vxlan :vni 100
        (ethernet
          (ipv4 :src "10.100.0.1/24"))))))
```

Spine: `ipv4 · udp · vxlan · ethernet · ipv4`

Candidates at position 0:

| Tile | Length | Guard |
|---|---|---|
| `VXLAN` | 4 | dport = 4789 ✓ |
| `FOU` | 2 | ✗ port not registered for generic encap |

Longest match takes `VXLAN`, consuming four nodes. The remaining `ipv4` matches no tile — correctly, because it is not another device. It is **the address on this one**. That is the terminal rule: a leftover L3 node bearing an address becomes `ip addr add`.

```bash
ip link add vxlan100 type vxlan \
    id 100 local 192.168.1.10 remote 192.168.1.20 \
    dstport 4789 dev eth0
ip addr add 10.100.0.1/24 dev vxlan100
ip link set vxlan100 up
```

> **Why `dstport` must never be omitted.** `iproute2`'s default VXLAN destination port is **8472**, the pre-standard Linux value, not the IANA-assigned 4789. A compiler that silently dropped the parameter would produce a device that comes up perfectly clean and talks to nothing. This single fact justifies the "omission is an error" rule in §6.

### 4.2 The inner node selects the primitive

Two trees, identical for their first two nodes:

```lisp
(ipv4 :src A :dst B (gre (ipv4 :src "10.20.0.1/30")))
(ipv4 :src A :dst B (gre (ethernet (ipv4 :src "10.50.0.1/24"))))
```

```
GRE     ipv4 · gre · ipv4        length 3
GRETAP  ipv4 · gre · ethernet    length 3
```

Equal length, but the patterns are disjoint at the third position, so exactly one matches either tree. No priority needed.

```bash
ip link add gre1 type gre local A remote B
ip addr add 10.20.0.1/30 dev gre1
```
```bash
ip link add gretap1 type gretap local A remote B
ip addr add 10.50.0.1/24 dev gretap1
```

This is the L2-over-L3 versus L3-over-L3 distinction doing real work: it selects a different kernel device.

### 4.3 Longest match finds the configuration/payload boundary

```lisp
(ipv4 :src A :dst B
  (ipv4 :src "10.30.0.1/30"
    (tcp :dst-port 22)))
```

Spine: `ipv4 · ipv4 · tcp`. The `IPIP` tile consumes two nodes; `tcp` is left over.

It **must** be left over. TCP is traffic flowing *through* the tunnel, not configuration *of* it. Maximal munch drew the line between "this is device config" and "this is payload" without being told where the line is.

```bash
ip tunnel add tun0 mode ipip local A remote B
ip addr add 10.30.0.1/30 dev tun0
```

Same tile set, different next header:

```lisp
(ipv4 :src A :dst B (ipv6 :src "2001:db8::1/64"))
```
```bash
ip tunnel add sit1 mode sit local A remote B
```

### 4.4 Chained tiles: nested tunnels for free

VXLAN inside IPsec:

```lisp
(configure
  (ipv4 :src A :dst B
    (esp :spi "0x1001" :sa "site-a-to-b"
      (ipv4 :src "172.20.0.2"
        (udp :dst-port 4789
          (vxlan :vni 100
            (ethernet
              (ipv4 :src "10.100.0.1/24"))))))))
```

Spine: `ipv4 · esp · ipv4 · udp · vxlan · ethernet · ipv4`

```
pos 0   XFRM      matches ipv4 · esp                     length 2
pos 2   VXLAN     matches ipv4 · udp · vxlan · ethernet  length 4
pos 6   terminal  ipv4 → address
```

Two tiles cover one spine, and neither tile knows the other exists. This is the payoff of tiling over a lookup table of named stacks: **there is no `VXLANOverIPsec` case to write.**

```bash
ip link add xfrm0 type xfrm dev eth0 if_id 1
ip xfrm state add src A dst B proto esp spi 0x1001 ...
ip link add vxlan100 type vxlan id 100 \
    local 172.20.0.2 dstport 4789 dev xfrm0
```

Note `dev xfrm0` in the last command. **The outer tile's device becomes the inner tile's underlay.** Tiling produces both the ordering and the dependency edge automatically, which matters because these commands do not commute and a naive emitter produces a script that fails halfway through.

### 4.5 The star changes the emitted instruction

```lisp
(ipv4 :src "192.168.1.10" :dst *
  (udp :dst-port 4789
    (vxlan :vni 100 (ethernet ...))))
```

Same tile, different guard branch — one remote versus many:

```bash
ip link add vxlan100 type vxlan id 100 \
    local 192.168.1.10 group 239.1.1.1 dstport 4789 dev eth0
```

With the VNI dynamic as well:

```lisp
(ipv4 :src "192.168.1.10" :dst *
  (udp :dst-port 4789
    (vxlan :vni * (ethernet ...))))
```
```bash
ip link add vxlan0 type vxlan external dstport 4789 dev eth0
```

`external` is collect-metadata mode: one device serving all VNIs, driven by route metadata. It is how OVS and Cilium actually run VXLAN, and in this language it is simply what `:vni *` means.

---

## 5. The real design problem: an instance is not a class

The tiling is the easy part. This is the hard part, and it deserves the design effort.

`(ipv4 :src "10.100.0.1" :dst "10.100.0.2" ...)` describes **one packet** between two endpoints.

`ip link add vxlan100 ...` creates a **device handling a whole class of packets** — every VNI-100 frame to any peer.

So the lowering keeps the local address and silently discards the destination, because a device has one local address and potentially many remotes. **This is not compilation. It is generalization from a single example.** One sample underdetermines the device, and several different generalizations are all consistent with it:

- point-to-point (`remote`)
- multicast (`group`)
- control-plane learning (EVPN)

The packet looks identical in all three cases.

### The star is the fix, and it is load-bearing

`*` in `configure` does **not** mean "don't care." It means **"this field varies across the packets this device handles."** That is a different concept from the wildcard in `receive`, where absence means "match anything," and conflating them will produce devices that come up clean and blackhole traffic.

The interpretation is per-field, which is the honest situation:

| Field | `*` lowers to |
|---|---|
| outer `:dst` | `group` / EVPN instead of `remote` |
| outer `:src` | let the kernel choose via `dev` |
| `:vni` | `external` (collect-metadata) mode |
| inner MACs | normal learning (the default) |

The star is not a syntactic convenience. It maps onto real kernel modes that have no other natural expression in a packet-shaped language.

### Naming note

`*` reads as a glob, which suggests matching. If "varies" and "unconstrained" ever need to be distinguished, `?` or `$dynamic` leaves room. Not worth blocking on.

---

## 6. Determinism

The compiler is deterministic given four rules.

1. **Longest match wins.**
2. **Ties break on declared priority**, and the tile set is validated at load time for unresolvable ties (two tiles sharing pattern, length, and priority is a startup error, not a lowering error).
3. **`*`, concrete, and omitted are three distinct states. Omitted is an error.** No silent defaults, ever.
4. **No tile means no output.** Never a partial script.

### Where ambiguity actually arises

**Same prefix, different protocol — resolved by guard.** `ipv4 · udp · X` where X is VXLAN (4789), Geneve (6081), L2TP (1701), WireGuard (51820), or MPLS-over-UDP (6635). The port guard makes these mutually exclusive. Deterministic.

**Unbound port — must refuse.**

```lisp
(ipv4 (udp :dst-port * (vxlan :vni 100 ...)))
```

The VXLAN node names the protocol, so the port *could* be guessed. Don't. `:dst-port *` means the device serves multiple ports, which VXLAN cannot. Error.

**Multiple backends — not a tiling problem.** `ipv4·udp·vxlan·ethernet` could lower to a netdev VXLAN, an OVS port, or an eBPF program. That is target selection, not tile ambiguity. It belongs in the deployment layer alongside `host:` and `interface:`. Choose a backend first, then tile.

**Genuine overlap — this is what `Priority` is for.** FOU/GUE is the real case: `ip fou add port 5555 ipproto 4` builds a generic UDP encapsulation that can carry payloads a specific tile also handles. Two tiles legitimately match the same nodes at the same length.

### What must fail loudly

```lisp
(ethernet :ethertype "mpls-unicast"
  (mpls :labels (...)
    (pseudowire :type "ethernet" (ethernet ...))))
```

There is no mainline Linux device for an MPLS pseudowire. No tile matches. The compiler must report **"no lowering exists for this spine"** and emit nothing. It must not produce a partial script that creates half a thing.

Similarly, `esp :mode "transport"` is `ip xfrm policy` rather than a device at all. It either gets its own tile emitting policy commands, or it is rejected.

---

## 7. Facts the packet tree cannot carry

Look at the emitted commands and ask where each value came from:

```bash
ip addr add 10.100.0.1/24 dev vxlan100
                        ^^^      ^^^^^^^^
```

Neither the prefix length nor the interface name is in the packet. Packets do not carry netmasks or device names. **The lowering is not total.**

The full list of what the tree structurally cannot determine:

- prefix length
- device name
- MTU
- underlay `dev`
- VXLAN `learning` / `nolearning`
- `group` versus `remote`
- GRE key handling
- TTL and TOS inheritance policy

This is not fatal, and compilers have exactly the same situation — they call it a **calling convention**: the facts the expression tree doesn't determine but the target requires.

It needs an explicit home, and that home must not be inside the packet expression. The deployment layer already established for `host:` / `interface:` / `hook:` is where it goes.

---

## 8. Multi-host deployment

A tunnel needs configuration on both ends. The assumption is a control machine with SSH access to both.

### The assumption is valid

This is the Ansible push model. Cloud-native tooling went pull/agent — Cilium runs a daemon per node — because Kubernetes nodes are cattle behind NAT. For an edge fabric of OpenWrt boxes and SR Linux nodes, push is the realistic model.

### One tree generates both sides

A tunnel is symmetric, so the peer's configuration is the local one with `src` and `dst` exchanged:

```
Host A:  local 192.168.1.10  remote 192.168.1.20
Host B:  local 192.168.1.20  remote 192.168.1.10
```

This is the same duality as send/receive applied to configuration instead of packets: **one description, two evaluations, selected by which endpoint is being rendered.** The peer config comes for free, which is precisely the thing people get wrong by hand.

### But the mirror is not pure

These fields do not simply swap:

| Field | Behaviour under mirroring |
|---|---|
| VNI, GRE key, ESP SPI | identical on both sides |
| inner addresses | `10.100.0.1/24` ↔ `10.100.0.2/24` — same prefix, different host part |
| underlay `dev` | `eth0` on one host, `wan` on an OpenWrt box |
| interface name | may collide with pre-existing devices |
| WireGuard keys | genuinely asymmetric — each side needs the other's **public** key |

The tree supplies the symmetric part; the deployment layer supplies the rest. Same split as §7.

### Three risks

**Lockout.** If the underlay is the link you are SSH'd over, a bad configuration costs you the machine. This is the risk that actually bites. Mitigation is old and boring: apply behind a dead-man timer that reverts unless confirmed within N seconds. Build it early — it is needed during development, not just in production.

**Half-applied state.** Two machines, no distributed transaction, and you cannot two-phase commit over SSH. The practical answer is not rollback but **idempotence**: make every script safe to re-run, so recovery is "run it again" rather than "unwind it." A half-configured tunnel is inert rather than dangerous, which makes this survivable.

**NAT asymmetry.** If one side is behind NAT, the mirror is invalid — the NATed side cannot be named as a `remote`. VXLAN and GRE cannot do this at all; WireGuard can, because it learns the peer endpoint from the handshake. The compiler can detect this: if a tile requires a symmetric `remote` and the deployment layer marks one host as NATed, refuse. That is a genuinely useful error message.

---

## 9. Scope boundary

**Emit scripts. Do not execute them.**

One named script per host, readable before it runs, human-invoked.

The moment the tool is SSHing, sequencing, reconciling desired state against actual state, and handling partial failure, it has become a small Ansible, and the interesting part is behind it. The compiler is the contribution. The orchestration is not.

---

## 10. The loop this closes

The packet tree now has interpreters that check each other:

```
                packet tree
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
    generator    configure     dissector
        │            │
   expected       actual
    packet        device
        │            │
        └──── diff ──┘
```

Bring the configured interface up, capture what it actually emits, and diff against what the generator produced from the same tree. **When they disagree, one of the two interpreters is wrong**, and you have a test that no amount of documentation could replace.

No other thread in this design closes a loop like that. It is the strongest single argument for the whole approach.

---

## 11. Build order

1. **Spine extraction and the tile struct.** Everything rests here.
2. **Three tiles: IPIP, GRE, GRETAP.** Simplest lowerings, and GRE/GRETAP immediately exercises pattern disjointness.
3. **The tiler.** Forty lines. Longest match, priority tiebreak, load-time tile validation.
4. **VXLAN, including star semantics.** This is where the instance-versus-class problem becomes concrete.
5. **The mirror.** Generate the peer script. Get the asymmetric fields right.
6. **Chained tiles: XFRM plus VXLAN.** Proves nested tunnels need no special case.
7. **The diff loop.** Capture from a live device, compare against the generator.

Steps 1 through 4 are a weekend and produce something genuinely usable. Steps 5 through 7 are what make it worth writing about.

---

## 12. Assessment

**Pursue it.** The reasons, in order of weight:

- **It is finishable.** Six tiles, a forty-line algorithm, output that is a shell script you can read before running. No runtime, no correlation problem, no operational story required.
- **The framing is uncommon.** Every existing tool names the mechanism first. Describing the artifact and deriving the mechanism inverts that, and the inversion is the interesting part.
- **It closes a verification loop** that nothing else in the design closes.
- **It is independent.** It does not block, and is not blocked by, the pedagogical article or the distributed observability fabric.

The caution: **the generalization problem in §5 is the actual work, not the tiling.** One packet underdetermines a device; `*` is the patch; getting star semantics right per field is where the design either holds up or gets muddy. Budget thinking there, not on the tile matcher.

---

# Appendix: what the implementation changed

This report was written before any code existed. Building it confirmed
most of the design and corrected some of it. Rather than quietly editing
the text above, the differences are recorded here.

## Confirmed against a real kernel

**The VXLAN port default is 8472** (§4.1). Verified in a network
namespace; `iproute2` warns about it unprompted. This is why an omitted
`:dst-port` is an error rather than a default, and the diagnostic says
so.

**`external` and `id` are mutually exclusive** (§4.5). The kernel
rejects them together, which is the same exclusion the `*` states
already express. Pinned as a test.

**Tiling is deterministic under the four rules** (§6). The spine does
not branch, so greedy longest-match is a single pass over a list. The
tiler is under fifty lines.

**Chained tiles need no combined tile** (§4.4). Two tiles cover one
nested spine and neither knows the other exists. The outer tile's
device becomes the inner tile's underlay through one field on `Env`.

## Changed

**The star does not decide multicast.** §5 proposed `:dst *` lowering to
`group`. In practice a multicast outer destination is *concrete* — the
group address really is the destination IP on the wire — so
`netip.Addr.IsMulticast()` selects `group` over `remote`, and `:dst *`
means something else entirely: peers are learned rather than
configured. This keeps the fact in the packet instead of leaking it into
the deployment layer, which is better than what the report proposed.

**Interface naming is by device type, not `tun0`.** `gre1`, `gretap1`,
`ipip1`, and `vxlan100` named after its VNI. Self-documenting, and
collisions across rules are a load-time error.

**`ip link add ... type X` throughout**, rather than the report's mix of
`ip link add` and `ip tunnel add`.

**A prefix on an intermediate layer means two things.** Not anticipated
by the report. `:src "10.100.0.1/24"` between two tunnels is both the
inner tunnel's local endpoint and an address the outer device needs
before anything can stand on it. See `tiles/linux/endpoint.go`.

**The mirror turns around every two-ended layer**, not just the
outermost (§8). A nested stack has more than one. Prefix lengths stay
with whichever field is local, so `10.100.0.1/24` mirrors to
`10.100.0.2/24` rather than the length travelling with the address.
This is also the one place a peer's inner address can be derived rather
than supplied, because both host parts are already written down.

## Added

**Wire prediction.** Not in the report. A third reading of the same
tree: what the underlay should carry once the configuration is running.
It closes the loop §10 describes, and it is checked end to end — two
namespaces, both ends configured from one description, traffic pushed
through, underlay captured, prediction compared. See `wire/`.

## Deferred, and why

**One `configure` form per file.** Several forms error out rather than
doing something surprising. `Env` already handles name collisions across
rules, so this is mostly plumbing.

**Four tiles.** IPIP, GRE, GRETAP, VXLAN. Geneve, VLAN, XFRM and
WireGuard are absent for one reason: their kernel modules were not
available in the environment where the tiles were written, and shipping
syntax that has never been run is how a compiler acquires quiet bugs.

**One backend.** The `Tile`/`Lower` split described in the report is not
yet a `Lowerer` interface, because designing that abstraction against a
single implementation would design it badly. The right moment is the
second backend.

**No execution.** Unchanged from §9, and not a temporary state. `pktc`
prints scripts. The moment it applies them it is a configuration
management tool, and the interesting part is behind it.
