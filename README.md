# containers

Personal container images, published to `ghcr.io/ronaldphilipsen/<app>`.

The layout and CI approach follow
[home-operations/containers](https://github.com/home-operations/containers),
scaled down for a personal repository.

## Images

| App | Image | Description |
| --- | ----- | ----------- |
| [db-bridge](apps/db-bridge) | `ghcr.io/ronaldphilipsen/db-bridge` | spiffe-helper (SPIFFE SVID fetch and rotation) plus a small Go mTLS Postgres bridge, on `scratch` (no shell; db-bridge is PID 1 and supervises spiffe-helper) |

## Layout

```text
apps/<app>/
  Dockerfile        # the image
  docker-bake.hcl   # APP, VERSION, SOURCE, platforms
  test.sh           # optional smoke test: test.sh <image>
```

## Tags

Each release pushes one tag: the `VERSION` from `apps/<app>/docker-bake.hcl`.
There are no floating tags (`latest`, `rolling`, `1`, `1.1`).

CI never overwrites a tag. Before it pushes, CI lists the tags in GHCR. If
`VERSION` already exists, CI builds and tests the image but does not push it,
and it writes a warning. CI checks again just before the push. If the tag
appeared during the build, the job fails.

Pin images by digest, for example:

```yaml
image: ghcr.io/ronaldphilipsen/db-bridge:1.1.3@sha256:<digest>
```

## Release an app

1. Change files in `apps/<app>/`.
2. Set `VERSION` in `apps/<app>/docker-bake.hcl` to a version that is not in GHCR yet.
3. Merge to `main`. The `Release` workflow builds every changed app, pushes the
   new tag, reads the digest back from the registry, checks that the image can
   be pulled without credentials, and adds a build provenance attestation.

To build again by hand, run the `Release` workflow with `workflow_dispatch`
(`app`, `release`, `no-cache`).

## Workflows

- `release.yaml`: on push to `main` (paths `apps/**`) or by hand. Finds changed
  apps and calls `app-builder.yaml` with `release: true`.
- `pull-request.yaml`: on pull requests. Builds and tests changed apps without
  pushing, and runs shellcheck and zizmor.
- `app-builder.yaml`: reusable. It reads `docker-bake.hcl`, checks the tag,
  builds the image with `docker buildx bake`, and runs `test.sh`. On a release,
  if the tag is new, it pushes the image.

## Renovate

`.renovaterc.json5` pins base images and GitHub Actions by digest, and reads
`# renovate:` comments above `ARG` lines in Dockerfiles and above variables in
`docker-bake.hcl`.

When Renovate bumps a download version (for example `SPIFFE_HELPER_VERSION`),
also update its `*_SHA256` ARG and the app `VERSION` in the same pull request.
If the checksum is stale, the build fails at `sha256sum -c`.

## Local build

```sh
cd apps/db-bridge
docker buildx bake image-local
./test.sh db-bridge:1.1.3
```
