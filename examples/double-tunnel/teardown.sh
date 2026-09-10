#!/bin/sh
ip netns del A 2>/dev/null || true
ip netns del B 2>/dev/null || true
echo "gone"
