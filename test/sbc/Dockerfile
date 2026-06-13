FROM ghcr.io/kamailio/kamailio-ci:5.8

COPY kamailio.cfg /etc/kamailio/kamailio.cfg

EXPOSE 5060/udp 5060/tcp

CMD ["kamailio", "-DD", "-E", "-e"]
