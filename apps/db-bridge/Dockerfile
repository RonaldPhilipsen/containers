# syntax=docker/dockerfile:1

# db-bridge: SPIFFE-aware Postgres bridge sidecar.
#
# db-bridge (PID 1) runs spiffe-helper (fetches + rotates the pod's X.509 SVID from the SPIRE
# agent via the CSI-mounted Workload API socket) and the db-bridge binary
# (a small mTLS Postgres bridge: plain TCP on loopback, SVID client cert to
# CloudNativePG). One container so SVID rotation is absorbed in place — the
# bridge re-reads the SVID files on every new connection, no app-visible
# restart.
#
# The bridge config is identical for every app (same CNPG rw upstream; the
# SVID carries the identity), so nothing app-specific is mounted.
# DB_BRIDGE_UPSTREAM (host:port, default postgres-rw.database.svc:5432)
# selects the CNPG rw service; its host is the TLS server name.
# DB_BRIDGE_LISTEN (default 127.0.0.1:5432) is where the app connects.
# Volumes expected in the pod:
#   spiffe   : csi driver csi.spiffe.io -> /run/spiffe (agent socket)
#   svid     : emptyDir                 -> /spiffe    (svid.crt/svid.key/bundle.crt)
#   cnpg-ca  : secret (server CA)       -> /cnpg-ca   (ca.crt)
FROM docker.io/library/golang:1.27-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS build
WORKDIR /src
COPY bridge/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/db-bridge .

FROM docker.io/library/debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 AS spiffe-helper
# renovate: datasource=github-releases depName=spiffe/spiffe-helper
ARG SPIFFE_HELPER_VERSION=v0.12.3
# sha256 of spiffe-helper_${SPIFFE_HELPER_VERSION}_Linux-x86_64.tar.gz.
# Update together with SPIFFE_HELPER_VERSION.
ARG SPIFFE_HELPER_SHA256=b275a572d36ec02f5da995c11c736b3300ab5210af21f3deff1b5fc727cecd69
RUN \
    apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && curl -fsSL --retry 5 --retry-all-errors --retry-delay 5 \
        -o /tmp/spiffe-helper.tar.gz \
        "https://github.com/spiffe/spiffe-helper/releases/download/${SPIFFE_HELPER_VERSION}/spiffe-helper_${SPIFFE_HELPER_VERSION}_Linux-x86_64.tar.gz" \
    && echo "${SPIFFE_HELPER_SHA256}  /tmp/spiffe-helper.tar.gz" | sha256sum -c - \
    && tar -xzf /tmp/spiffe-helper.tar.gz -C /usr/local/bin spiffe-helper \
    && chmod 0755 /usr/local/bin/spiffe-helper

# Final image: no shell, no libc, no package manager. Both binaries are
# static Go builds, and db-bridge is PID 1 (it supervises spiffe-helper).
FROM scratch

COPY --from=spiffe-helper /usr/local/bin/spiffe-helper /usr/local/bin/spiffe-helper
COPY --from=build /out/db-bridge /usr/local/bin/db-bridge
COPY helper.conf /etc/spiffe-helper/helper.conf

# Explicit PATH: exec probes such as "db-bridge -health" resolve through it.
ENV PATH=/usr/local/bin

# Non-root by default; the pod's securityContext can override it.
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/db-bridge"]
