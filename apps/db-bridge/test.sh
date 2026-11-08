#!/usr/bin/env bash
# Smoke test for the db-bridge image. Usage: test.sh <image>
set -euo pipefail
IMAGE="$1"
docker run --rm --entrypoint sh "$IMAGE" -c '
    set -eu
    test -x /entrypoint.sh
    test -f /etc/spiffe-helper/helper.conf
    test -x /usr/local/bin/db-bridge
    spiffe-helper -version
'
echo "db-bridge smoke test passed"
