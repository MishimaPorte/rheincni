FROM scratch

COPY rheindaemon /usr/local/bin/rheindaemon

ENTRYPOINT ["/usr/local/bin/rheindaemon"]
