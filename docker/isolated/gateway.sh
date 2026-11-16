#!/bin/sh
# Gateway of an isolated environment (ADR 0004). It holds the network
# namespace the workspace container joins, and installs the only egress rule:
# the internet is open, the host and every local network are not. The
# workspace has no capabilities, so it cannot change these rules.
set -eu
for table in iptables ip6tables; do
  $table -F OUTPUT
  $table -A OUTPUT -o lo -j ACCEPT
  $table -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
done
for net in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16 100.64.0.0/10 0.0.0.0/8; do
  iptables -A OUTPUT -d "$net" -j REJECT
done
# No IPv6 egress until the same ranges are mapped for it.
ip6tables -A OUTPUT -j REJECT
touch /tmp/gateway-ready
echo "gateway ready"
exec sleep infinity
