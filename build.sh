#!/usr/bin/env bash

# Usage:
#   sh build.sh <tag> [GOOS] [GOARCH]
# Examples:
#   sh build.sh v1.0.0
#   sh build.sh v1.0.0 linux arm64
#   sh build.sh v1.0.0 linux amd64

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Please Input Build Args : sh build.sh {{tag}} [GOOS] [GOARCH]"
  exit 1
fi

BUILD_TAG="$1"
GOOS="${2:-linux}"
GOARCH="${3:-amd64}"

echo "Build Tag:        $BUILD_TAG"
echo "Target GOOS:      $GOOS"
echo "Target GOARCH:    $GOARCH"

BUILD_PATH="$(pwd)"
echo "Build Path:       $BUILD_PATH"

GOLANG_VERSION="alpine"
echo "Golang Version:   $GOLANG_VERSION"

CONTAINER_WORKDIR="/workspace"
HOST_OUTPUT_DIR="$BUILD_PATH/build/bin"
CONTAINER_OUTPUT_DIR="$CONTAINER_WORKDIR/build/bin"
OUTPUT_NAME="${OUTPUT_NAME:-main}"
HOST_BINARY="$HOST_OUTPUT_DIR/$OUTPUT_NAME"
CONTAINER_BINARY="$CONTAINER_OUTPUT_DIR/$OUTPUT_NAME"

mkdir -p "$HOST_OUTPUT_DIR"

# 1) build binary inside golang container
docker run --rm \
  -v "$BUILD_PATH":"$CONTAINER_WORKDIR" \
  -w "$CONTAINER_WORKDIR" \
  -e CGO_ENABLED=0 \
  -e GOOS="$GOOS" \
  -e GOARCH="$GOARCH" \
  golang:"$GOLANG_VERSION" \
  go build -v -a -installsuffix cgo -o "$CONTAINER_BINARY" .

cd "$BUILD_PATH"

IMAGE_NAME="iwdmb/get-container-id:$BUILD_TAG"
echo "Docker Image:     $IMAGE_NAME"

# 2) build docker image (try buildx for cross-platform)
if docker buildx version >/dev/null 2>&1; then
  echo "docker buildx detected, building for linux/$GOARCH ..."
  docker buildx build \
    --platform "linux/$GOARCH" \
    -t "$IMAGE_NAME" \
    --load \
    .
else
  echo "docker buildx not found, using docker build (host arch)..."
  docker build -t "$IMAGE_NAME" .
fi

# 3) cleanup
rm -f "$BUILD_PATH/build/bin/main"

# 4) run for quick test (only if target is linux/amd64 or linux/arm64 and host can run it)
if [ "$GOOS" = "linux" ]; then
  echo "Running container for quick test on port 8080..."
  docker run -it --rm "$IMAGE_NAME" -httpPort 8080
else
  echo "Target GOOS=$GOOS (not linux), skipping run step."
fi
