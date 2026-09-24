FROM scratch

COPY rheincni-ipam /usr/local/bin/rheincni-ipam

ENTRYPOINT ["/usr/local/bin/rheincni-ipam"]
