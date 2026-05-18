# Runtime-only image. Build linux/amd64 binaries before podman build so the
# test ECS does not depend on Docker Hub/GCR for builder or distroless bases.
FROM scratch
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY partner-api /usr/local/bin/partner-api
COPY outbox-poller /usr/local/bin/outbox-poller
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/partner-api"]
