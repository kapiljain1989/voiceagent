#!/bin/sh
# Replace gateway host placeholder with env var or default
GATEWAY_HOST="${GATEWAY_HOST:-host.docker.internal}"
sed -i "s/__GATEWAY_HOST__/${GATEWAY_HOST}/g" /etc/kamailio/kamailio.cfg
echo "SBC routing calls to gateway: ${GATEWAY_HOST}:5062"
exec kamailio -DD -E -f /etc/kamailio/kamailio.cfg
