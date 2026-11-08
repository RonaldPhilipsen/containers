#!/usr/bin/env bash
# Smoke test for the omada-cert-sync image. Usage: test.sh <image>
set -euo pipefail
IMAGE="$1"

# Without configuration, the job must fail and name the missing variable.
if OUTPUT=$(docker run --rm "$IMAGE" 2>&1); then
    echo "expected omada-cert-sync to fail without configuration" >&2
    exit 1
fi
grep -q 'OMADA_URL' <<<"$OUTPUT"

echo "omada-cert-sync smoke test passed"
