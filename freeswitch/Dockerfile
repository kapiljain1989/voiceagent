FROM drachtio/drachtio-freeswitch-mrf:latest

# Overlay our SBC-ready config onto the drachtio image's config directory.
COPY config/sip_profiles/external.xml /usr/local/freeswitch/conf/sip_profiles/external.xml
COPY config/dialplan/public.xml /usr/local/freeswitch/conf/dialplan/public.xml
COPY config/dialplan/outbound.xml /usr/local/freeswitch/conf/dialplan/outbound.xml
COPY config/autoload_configs/acl.conf.xml /usr/local/freeswitch/conf/autoload_configs/acl.conf.xml
COPY config/autoload_configs/event_socket.conf.xml /usr/local/freeswitch/conf/autoload_configs/event_socket.conf.xml
COPY config/autoload_configs/switch.conf.xml /usr/local/freeswitch/conf/autoload_configs/switch.conf.xml
COPY config/vars.xml /usr/local/freeswitch/conf/vars.xml

# Entrypoint injects SBC env vars into config at startup
COPY entrypoint.sh /entrypoint.sh

EXPOSE 5060/tcp 5060/udp
EXPOSE 16000-16020/udp
EXPOSE 8022/tcp

ENTRYPOINT ["/entrypoint.sh"]
