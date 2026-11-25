# syntax=docker/dockerfile:1@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183

# omada-cert-sync: pushes a mounted TLS certificate to an Omada controller.
#
# The job compares the certificate that the controller serves with the mounted
# one, and uploads only when they differ.
#
# Environment: OMADA_URL, OMADA_USER, OMADA_PASS, optional TLS_DIR (default /tls).
FROM docker.io/library/golang:1.27@sha256:5f1b98fa02fe396ebce2a0227a0e08309e11792915b9dd2e1f71053d7e7e6666 AS build
WORKDIR /src
COPY sync/ ./
RUN go test ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/omada-cert-sync .

FROM gcr.io/distroless/static:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5
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
