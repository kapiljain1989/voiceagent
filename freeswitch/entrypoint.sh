#!/bin/bash
# Inject SBC configuration from environment variables into vars.xml.
# This runs at container startup before FreeSWITCH.

VARS_FILE="/usr/local/freeswitch/conf/vars.xml"

if [ -f "$VARS_FILE" ]; then
  [ -n "$SBC_ADDRESS" ]    && sed -i "s|sbc_address=127.0.0.1|sbc_address=${SBC_ADDRESS}|" "$VARS_FILE"
  [ -n "$SBC_REGISTER" ]   && sed -i "s|sbc_register=false|sbc_register=${SBC_REGISTER}|" "$VARS_FILE"
  [ -n "$SBC_USERNAME" ]   && sed -i "s|sbc_username=voiceagent|sbc_username=${SBC_USERNAME}|" "$VARS_FILE"
  [ -n "$SBC_PASSWORD" ]   && sed -i "s|sbc_password=changeme|sbc_password=${SBC_PASSWORD}|" "$VARS_FILE"
  [ -n "$SBC_AUTH_CALLS" ] && sed -i "s|sbc_auth_calls=false|sbc_auth_calls=${SBC_AUTH_CALLS}|" "$VARS_FILE"
  [ -n "$EXTERNAL_RTP_IP" ] && sed -i "s|external_rtp_ip=\$\${local_ip_v4}|external_rtp_ip=${EXTERNAL_RTP_IP}|" "$VARS_FILE"
  [ -n "$EXTERNAL_SIP_IP" ] && sed -i "s|external_sip_ip=\$\${local_ip_v4}|external_sip_ip=${EXTERNAL_SIP_IP}|" "$VARS_FILE"
  # Inject ext_ip for docker-compose local mode
  if [ -n "$EXT_IP" ]; then
    grep -q "ext_ip" "$VARS_FILE" || \
      sed -i "/<\/include>/i \\  <X-PRE-PROCESS cmd=\"set\" data=\"ext_ip=${EXT_IP}\"/>" "$VARS_FILE"
  fi
fi

exec freeswitch -nonat -nf "$@"
