# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# omada-cert-sync: pushes a mounted TLS certificate to an Omada controller.
#
# The job compares the certificate that the controller serves with the mounted
# one, and uploads only when they differ.
#
# Environment: OMADA_URL, OMADA_USER, OMADA_PASS, optional TLS_DIR (default /tls).
FROM docker.io/library/golang:1.27@sha256:ec47ea3725fea0cc88f6de94919aef19f40f12fc1ec0e2d1b3426dbf15ecca04 AS build
WORKDIR /src
COPY sync/ ./
RUN go test ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/omada-cert-sync .

FROM gcr.io/distroless/static:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931
COPY --from=build /out/omada-cert-sync /usr/local/bin/omada-cert-sync
ENTRYPOINT ["/usr/local/bin/omada-cert-sync"]

# OCI image labels. Keep this block last so that changed values only rebuild this step.
ARG VERSION=devel
LABEL \
    org.opencontainers.image.title="omada-cert-sync" \
    org.opencontainers.image.description="Pushes a mounted TLS certificate to an Omada controller" \
    org.opencontainers.image.url="https://github.com/RonaldPhilipsen/containers" \
    org.opencontainers.image.source="https://github.com/RonaldPhilipsen/containers" \
    org.opencontainers.image.version="${VERSION}"
