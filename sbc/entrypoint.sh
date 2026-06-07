#!/bin/sh
# Inject EXT_IP into kamailio.cfg at container startup

CFG="/etc/kamailio/kamailio.cfg"

if [ -n "$EXT_IP" ]; then
    sed -i "s|__EXT_IP__|${EXT_IP}|g" "$CFG"
    echo "Kamailio: advertised_address set to ${EXT_IP}"
else
    CONTAINER_IP=$(ip -4 addr show eth0 2>/dev/null | grep -oP 'inet \K[\d.]+' || echo "0.0.0.0")
    sed -i "s|__EXT_IP__|${CONTAINER_IP}|g" "$CFG"
    echo "Kamailio: advertised_address set to ${CONTAINER_IP} (auto-detected)"
fi

exec kamailio -DD -E -f "$CFG"
