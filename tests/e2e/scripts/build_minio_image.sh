#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 0 ]]; then
    echo "Usage: $0" >&2
    exit 2
fi

for tool in git podman python3; do
    command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done

# Match the Bitnami source revision used by Velero's Kind E2E workflow.
bitnami_commit=19fb570e551f15ab0c8264aafa93774266761b8d
image=quay.io/migtools/minio
tag=bitnami-2026.7.17-debian-12-r0
workdir=$(mktemp -d)
checkout="$workdir/containers"
source_dir="$checkout/bitnami/minio/2026/debian-12"
manifest="localhost/oadp-minio-build:${tag}-$$"

echo "Build directory: $workdir"
git init -q "$checkout"
git -C "$checkout" remote add origin https://github.com/bitnami/containers.git
git -C "$checkout" sparse-checkout init --cone
git -C "$checkout" sparse-checkout set bitnami/minio/2026/debian-12
git -C "$checkout" fetch --depth 1 --filter=blob:none origin "$bitnami_commit"
git -C "$checkout" checkout -q --detach FETCH_HEAD

# Podman cannot parse Docker's optional BuildKit secret mount. The build uses
# Bitnami's default download URL and still checks each archive's SHA256 hash.
python3 - "$source_dir/Dockerfile" "$source_dir/Dockerfile.podman" <<'PY'
from pathlib import Path
import sys

source = Path(sys.argv[1]).read_text()
mount = "RUN --mount=type=secret,id=downloads_url,env=SECRET_DOWNLOADS_URL "
if source.count(mount) != 1:
    raise SystemExit("expected one optional Bitnami download secret mount")
Path(sys.argv[2]).write_text(source.replace(mount, "RUN ", 1))
PY

podman build --format docker --platform linux/amd64 \
    -t "$image:$tag" -f "$source_dir/Dockerfile.podman" "$source_dir"
podman build --format docker --platform linux/arm64 --build-arg TARGETARCH=arm64 \
    -t "$image:$tag-arm64" -f "$source_dir/Dockerfile.podman" "$source_dir"
podman manifest create "$manifest" "$image:$tag" "$image:$tag-arm64"
echo "Built local multiarch manifest $manifest"
echo "After validation, publish this exact manifest with:"
echo "podman manifest push --all --digestfile /tmp/oadp-minio-digest $manifest docker://$image:$tag"
