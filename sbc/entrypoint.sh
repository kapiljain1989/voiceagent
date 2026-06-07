#!/bin/bash
# Inject EXT_IP into kamailio.cfg at container startup

CFG="/etc/kamailio/kamailio.cfg"

if [ -n "$EXT_IP" ]; then
    sed -i "s|__EXT_IP__|${EXT_IP}|g" "$CFG"
    echo "Kamailio: advertised_address set to ${EXT_IP}"
else
    # Auto-detect container IP as fallback
    CONTAINER_IP=$(hostname -i | awk '{print $1}')
    sed -i "s|__EXT_IP__|${CONTAINER_IP}|g" "$CFG"
    echo "Kamailio: advertised_address set to ${CONTAINER_IP} (auto-detected)"
fi

exec kamailio -DD -E -f "$CFG"
