#!/bin/sh
# Generate both ends from one description, apply them, and watch the
# underlay while traffic runs on the innermost network.
set -e
PKTC="go run ./cmd/pktc"

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
