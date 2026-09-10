# pktc

Describe the packet you expect on the wire; pktc derives the Linux
configuration required to produce it.

    (configure
      (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
        (udp :dst-port 4789
          (vxlan :vni 100
            (ethernet
              (ipv4 :src "10.100.0.1/24"))))))

lowers to

    ip link add vxlan100 type vxlan id 100 \
        local 192.168.1.10 remote 192.168.1.20 dstport 4789 dev eth0
    ip addr add 10.100.0.1/24 dev vxlan100
    ip link set vxlan100 up

Every other tool makes you name the mechanism first. This one infers it
from the packet shape, by tree tiling — the same technique a compiler
back end uses for instruction selection.

pktc emits scripts. It does not run them.

Status: early. Not production.
