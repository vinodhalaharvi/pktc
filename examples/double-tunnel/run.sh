#!/bin/sh
#
# RUN THIS INSIDE THE LIMA VM, NOT ON THE MAC.
#
#   macOS      → limactl shell pktc
#   in the VM  → cd /Users/<you>/go-projects/pktc
#                sudo sh examples/double-tunnel/setup.sh
#                sh examples/double-tunnel/run.sh
#
# macOS has no network namespaces, no ip(8) and no VXLAN devices, so
# none of this works there. Editing and git stay on the Mac: the Lima
# mount is read-only, so `git pull` inside the VM will fail.
#
# Generates both ends of the tunnel from one description, applies them,
# and captures the underlay while traffic runs on the innermost network.

set -e

# Work from this script's directory so it runs from anywhere, and find
# the repo root two levels up.
cd "$(dirname "$0")"
REPO=$(cd ../.. && pwd)
PKTC="go run $REPO/cmd/pktc"

$PKTC lower -quiet -underlay vethA double.lisp > /tmp/A.sh
$PKTC mirror -peer-inner 10.200.0.2/24 -peer-underlay vethB double.lisp \
  | sed -n '/far end/,$p' | grep '^ip ' > /tmp/B.sh

echo "--- near end (A) ---"; cat /tmp/A.sh
echo "--- far end (B) ----"; cat /tmp/B.sh

sudo ip netns exec A sh -e /tmp/A.sh
sudo ip netns exec B sh -e /tmp/B.sh

# Warm the inner tunnel's forwarding table so the capture shows data
# rather than the ARP that precedes it.
sudo ip netns exec A bash -c 'echo warm > /dev/udp/10.200.0.2/9999 2>/dev/null' || true
sleep 1

echo "--- what the underlay carries ---"
sudo ip netns exec B timeout 6 tcpdump -i vethB -c 1 -nn -v udp port 4789 2>/dev/null &
sleep 1
sudo ip netns exec A bash -c 'for i in 1 2 3; do echo payload > /dev/udp/10.200.0.2/9999 2>/dev/null; sleep 0.4; done'
wait
