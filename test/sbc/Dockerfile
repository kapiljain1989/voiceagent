FROM kamailio/kamailio:5.7.3-bullseye

COPY kamailio.cfg /etc/kamailio/kamailio.cfg
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Fix module path for the actual architecture
RUN MPATH=$(find /usr/lib -name "tm.so" -path "*/kamailio/*" 2>/dev/null | head -1 | xargs dirname) && \
    sed -i "s|mpath=.*|mpath=\"${MPATH}/\"|" /etc/kamailio/kamailio.cfg

EXPOSE 5060/udp 5060/tcp

ENTRYPOINT ["/entrypoint.sh"]
