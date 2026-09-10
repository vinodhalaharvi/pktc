#!/bin/sh
# Two namespaces joined by a veth pair: the smallest arrangement in
# which a tunnel is a tunnel rather than a device talking to itself.
set -e
ip netns del A 2>/dev/null || true
ip netns del B 2>/dev/null || true

ip netns add A
ip netns add B
ip link add vethA type veth peer name vethB
ip link set vethA netns A
ip link set vethB netns B

ip netns exec A sh -c 'ip addr add 192.168.1.10/24 dev vethA; ip link set vethA up; ip link set lo up'
ip netns exec B sh -c 'ip addr add 192.168.1.20/24 dev vethB; ip link set vethB up; ip link set lo up'

echo "namespaces A and B are up on 192.168.1.0/24"
