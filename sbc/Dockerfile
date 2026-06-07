FROM kamailio/kamailio:latest

COPY kamailio.cfg /etc/kamailio/kamailio.cfg
COPY dispatcher.list /etc/kamailio/dispatcher.list
COPY entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

EXPOSE 5060/udp 5060/tcp

ENTRYPOINT ["/entrypoint.sh"]
