target "docker-metadata-action" {}

variable "APP" {
  default = "db-bridge"
}

// First-party app: VERSION is the image version, bumped by hand.
// CI refuses to push a tag that already exists in GHCR.
variable "VERSION" {
  default = "1.1.3"
}

variable "SOURCE" {
  default = "https://github.com/RonaldPhilipsen/containers/tree/main/apps/db-bridge"
}

group "default" {
  targets = ["image-local"]
}

target "image" {
  inherits = ["docker-metadata-action"]
  args = {
    VERSION = "${VERSION}"
  }
  labels = {
    "org.opencontainers.image.source" = "https://github.com/RonaldPhilipsen/containers"
    "org.opencontainers.image.description" = "spiffe-helper + mTLS Postgres bridge sidecar"
  }
}

target "image-local" {
  inherits = ["image"]
  output = ["type=docker"]
  tags = ["${APP}:${VERSION}"]
}

target "image-all" {
  inherits = ["image"]
  // spiffe-helper is fetched as the Linux-x86_64 release asset.
  platforms = [
    "linux/amd64"
  ]
}
