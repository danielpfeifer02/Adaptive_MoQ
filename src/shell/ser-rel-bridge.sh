#!/bin/bash

set -x euo -pipefail

SERVER_NS="server_ns"
RELAY_NS="relay_ns"

SERVER_VETH="veth0"
RELAY_VETH="veth1"
SERVER_VETH_PEER="veth0-br"
RELAY_VETH_PEER="veth1-br"

SERVER_VETH_ADDR="192.168.10.1"
RELAY_VETH_ADDR="192.168.10.2"
BRIDGE_IP="192.168.10.10"
BRIDGE_NET_ADDR="192.168.10.0"

BRIDGE_INTERFACE="v-net-0"
BRIDGE_VETH_TO_REL="veth1-br"

if [[ $EUID -ne 0 ]]; then
    echo "You must be root to run this script"
    exit 1
fi

# Remove server namespace if it exists
if ip netns list | grep -q "${SERVER_NS}"; then
    echo "deleting ${SERVER_NS}"
    ip netns del ${SERVER_NS}
fi
# Remove relay namespace if it exists
if ip netns list | grep -q "${RELAY_NS}"; then
    echo "deleting ${RELAY_NS}"
    ip netns del ${RELAY_NS}
fi
# Remove bridge interface if it exists
if ip link show | grep -q "${BRIDGE_INTERFACE}"; then
    echo "deleting ${BRIDGE_INTERFACE}"
    ip link set dev ${BRIDGE_INTERFACE} down
    brctl delbr ${BRIDGE_INTERFACE}
fi
# Remove server veth counterpart in bridge if it exists
if ip link show | grep -q "${SERVER_VETH_PEER}"; then
    echo "deleting ${SERVER_VETH_PEER}"
    ip link set dev ${SERVER_VETH_PEER} down
    ip link del ${SERVER_VETH_PEER}
fi
# Remove relay veth counterpart in bridge if it exists
if ip link show | grep -q "${RELAY_VETH_PEER}"; then
    echo "deleting ${RELAY_VETH_PEER}"
    ip link set dev ${RELAY_VETH_PEER} down
    ip link del ${RELAY_VETH_PEER}
fi

# Create namespaces
ip netns add ${SERVER_NS}
ip netns add ${RELAY_NS}

#show namespace
ip netns show

# Create bridge
ip link add ${BRIDGE_INTERFACE} type bridge
ip link set dev ${BRIDGE_INTERFACE} up

# Create veth links to link namespaces to bridge
ip link add ${SERVER_VETH} type veth peer name ${SERVER_VETH_PEER}
ip link add ${RELAY_VETH} type veth peer name ${RELAY_VETH_PEER}

# Add veth links to namespaces and bridge
ip link set ${SERVER_VETH} netns ${SERVER_NS}
ip link set ${SERVER_VETH_PEER} master ${BRIDGE_INTERFACE}
ip link set ${RELAY_VETH} netns ${RELAY_NS}
ip link set ${RELAY_VETH_PEER} master ${BRIDGE_INTERFACE}

# Add addresses to veth links
ip -n ${SERVER_NS} addr add ${SERVER_VETH_ADDR}/24 dev ${SERVER_VETH}
ip -n ${RELAY_NS} addr add ${RELAY_VETH_ADDR}/24 dev ${RELAY_VETH}

# Set veth links up
ip -n ${SERVER_NS} link set ${SERVER_VETH} up
ip -n ${RELAY_NS} link set ${RELAY_VETH} up
ip link set ${SERVER_VETH_PEER} up
ip link set ${RELAY_VETH_PEER} up

# Set lo interfaces up
ip -n ${SERVER_NS} link set lo up
ip -n ${RELAY_NS} link set lo up

# Set default route for server towards bridge
ip addr add ${BRIDGE_IP}/24 dev ${BRIDGE_INTERFACE}
ip -n ${SERVER_NS} route add default via ${BRIDGE_IP}
ip -n ${RELAY_NS} route add default via ${BRIDGE_IP}

# Set NAT for bridge
iptables --table nat -A POSTROUTING -s ${BRIDGE_NET_ADDR}/24 -j MASQUERADE

# Make sure the ipv4 forwarding is enabled
echo 1 > /proc/sys/net/ipv4/ip_forward

# Set DNS server entries
ip netns exec ${SERVER_NS} mkdir -p /etc/netns/${SERVER_NS}
ip netns exec ${SERVER_NS} echo "nameserver 8.8.8.8" > /etc/netns/${SERVER_NS}/resolv.conf
ip netns exec ${SERVER_NS} echo "nameserver 8.8.4.4" >> /etc/netns/${SERVER_NS}/resolv.conf

ip netns exec ${RELAY_NS} mkdir -p /etc/netns/${RELAY_NS}
ip netns exec ${RELAY_NS} echo "nameserver 8.8.8.8" > /etc/netns/${RELAY_NS}/resolv.conf
ip netns exec ${RELAY_NS} echo "nameserver 8.8.4.4" >> /etc/netns/${RELAY_NS}/resolv.conf

# TODO: remove later (for now not that big of a deal)
iptables -P FORWARD ACCEPT

# Add a delay to the bridge connecting relay and client
tc qdisc add dev ${BRIDGE_VETH_TO_REL} root netem delay 5ms