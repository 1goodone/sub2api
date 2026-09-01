#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 IMAGE_TAG" >&2
    exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION="$(tr -d '\r\n' < "${REPO_ROOT}/backend/cmd/server/VERSION")"
COMMIT="$(git -C "${REPO_ROOT}" rev-parse --short HEAD)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

docker buildx build \
    --platform linux/amd64 \
    --load \
    --tag "$1" \
    --build-arg VERSION="${VERSION}" \
    --build-arg COMMIT="${COMMIT}" \
    --build-arg DATE="${BUILD_DATE}" \
    --build-arg VITE_ENABLE_KEY_PACKAGES=false \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    --file "${REPO_ROOT}/Dockerfile" \
    "${REPO_ROOT}"
