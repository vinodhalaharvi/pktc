#!/bin/sh
#
# RUN THIS INSIDE THE LIMA VM, WITH SUDO.
#
#   sudo sh examples/double-tunnel/teardown.sh
#
# Deleting the namespaces takes every device inside them with it.

ip netns del A 2>/dev/null || true
ip netns del B 2>/dev/null || true
echo "gone"
