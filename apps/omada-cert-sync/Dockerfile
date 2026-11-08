# syntax=docker/dockerfile:1

# omada-cert-sync: pushes a mounted TLS certificate to an Omada controller.
#
# The job compares the certificate that the controller serves with the mounted
# one, and uploads only when they differ.
#
# Environment: OMADA_URL, OMADA_USER, OMADA_PASS, optional TLS_DIR (default /tls).
FROM docker.io/library/golang:1.27@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b AS build
WORKDIR /src
COPY sync/ ./
RUN go test ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/omada-cert-sync .

FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
COPY --from=build /out/omada-cert-sync /usr/local/bin/omada-cert-sync
ENTRYPOINT ["/usr/local/bin/omada-cert-sync"]
