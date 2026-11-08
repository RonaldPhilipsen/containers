#!/usr/bin/env bash
# Smoke test for the db-bridge image. Usage: test.sh <image>
# The image is FROM scratch, so every check runs a binary directly.
set -euo pipefail
IMAGE="$1"

# spiffe-helper is present and runs (static binary, no libc needed)
docker run --rm --entrypoint /usr/local/bin/spiffe-helper "$IMAGE" -version

# db-bridge runs; -health must fail because nothing is listening
if docker run --rm "$IMAGE" -health; then
    echo "expected db-bridge -health to fail with no listener" >&2
    exit 1
fi

# PID 1 handles SIGTERM: with no SPIRE agent it waits for the SVID, and
# "docker stop" must stop spiffe-helper and exit promptly (well before the
# 10s kill), with a non-zero code because no SVID was ever served.
CID=$(docker run -d "$IMAGE")
sleep 3
START=$(date +%s)
docker stop -t 10 "$CID" >/dev/null
ELAPSED=$(( $(date +%s) - START ))
CODE=$(docker inspect -f '{{.State.ExitCode}}' "$CID")
LOGS=$(docker logs "$CID" 2>&1)
docker rm "$CID" >/dev/null
if [ "$ELAPSED" -ge 9 ] || [ "$CODE" -eq 0 ] || [ "$CODE" -eq 137 ]; then
    echo "SIGTERM not handled: stop took ${ELAPSED}s, exit code $CODE" >&2
    exit 1
fi
grep -q 'received terminated while waiting for the SVID' <<<"$LOGS"

# no shell in the image
if docker run --rm --entrypoint /bin/sh "$IMAGE" -c true 2>/dev/null; then
    echo "expected no /bin/sh in a scratch image" >&2
    exit 1
fi

echo "db-bridge smoke test passed"
