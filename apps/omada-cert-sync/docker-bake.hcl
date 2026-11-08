target "docker-metadata-action" {}

variable "APP" {
  default = "omada-cert-sync"
}

// First-party app: VERSION is the image version, bumped by hand.
// CI refuses to push a tag that already exists in GHCR.
variable "VERSION" {
  default = "0.1.0"
}

variable "SOURCE" {
  default = "https://github.com/RonaldPhilipsen/containers/tree/main/apps/omada-cert-sync"
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
    "org.opencontainers.image.description" = "Pushes a mounted TLS certificate to an Omada controller"
  }
}

target "image-local" {
  inherits = ["image"]
  output = ["type=docker"]
  tags = ["${APP}:${VERSION}"]
}

target "image-all" {
  inherits = ["image"]
  platforms = [
    "linux/amd64"
  ]
}
