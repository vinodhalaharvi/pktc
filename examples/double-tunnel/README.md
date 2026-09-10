# Double tunnel example

A VXLAN tunnel carried inside another VXLAN tunnel, configured on both
ends from one description, with a capture showing all three layers on
the wire.

## Where to run what

macOS has no network namespaces, no `ip(8)` and no VXLAN devices, so the
example needs a Linux VM. On an Apple Silicon Mac, Lima is the least
friction:

```
limactl create --name=pktc --cpus=2 --memory=4 template://ubuntu-lts
limactl start pktc
limactl shell pktc
```

Inside the VM, once:

```
sudo apt-get update
sudo apt-get install -y golang-go make tcpdump iproute2
```

From then on:

| Where | What |
|---|---|
| **macOS** | git pull, add, commit, push. Editing files. |
| **Lima VM** | `setup.sh`, `run.sh`, `teardown.sh`, `make netns` |

The same directory is visible in both, so a file written on the Mac
appears in the VM immediately. The Lima mount is **read-only**, so git
commands inside the VM fail with `Read-only file system` — that is
expected, not a broken checkout. `go run` still works, because Go's
build cache lives in the VM's own home rather than the mounted
directory.

## Running it

```
cd /Users/<you>/go-projects/pktc
sudo sh examples/double-tunnel/setup.sh
sh examples/double-tunnel/run.sh
```

## What to look for

`# tiles: VXLAN → VXLAN` in the generated header. One tile, matched
twice — there is no combined tile for a nested stack.

The capture, one packet three layers deep:

```
IP 192.168.1.10.48665 > 192.168.1.20.4789: VXLAN, vni 100
  IP 10.100.0.1.48665 > 10.100.0.2.4789: VXLAN, vni 200
    IP 10.200.0.1.54140 > 10.200.0.2.9999: UDP, length 8
```

The payload travelled `10.200.0.0/24`, which exists nowhere on the wire.

`ip addr add 10.100.0.1/24 dev vxlan100` sits *between* the two
`link add` commands rather than after them. That prefix is the address
the inner tunnel stands on, so it has to exist before the inner device
is created. These commands do not commute.

The middle layer names both ends, so the mirror derives the far side of
it unaided — and keeps the `/24` with whichever field is local rather
than letting the length travel with the address. Only `10.200.0.2/24`
had to be supplied, because the innermost layer names one end.

## Between runs

```
sudo sh examples/double-tunnel/teardown.sh
```

The devices persist until the namespaces are deleted, and a second run
without teardown hits `RTNETLINK answers: File exists`.
