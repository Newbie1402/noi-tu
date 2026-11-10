#!/bin/sh
set -eu

: "${NOITU_DATABASE_URL:?NOITU_DATABASE_URL is required}"
: "${NOITU_API_KEY:?NOITU_API_KEY is required}"

REPOSITORY=${REPOSITORY:-https://github.com/Newbie1402/noi-tu.git}
BRANCH=${BRANCH:-main}
SOURCE_DIR=${SOURCE_DIR:-/var/lib/noitu-deploy/source}
STATE_FILE=${STATE_FILE:-/var/lib/noitu-deploy/main.commit}
STACK_NAME=${STACK_NAME:-noitu}
IMAGE=${IMAGE:-noitu:latest}
POLL_SECONDS=${POLL_SECONDS:-60}
export NOITU_DATABASE_URL NOITU_API_KEY STACK_NAME IMAGE
export NOITU_PORT=${NOITU_PORT:-18083}

mkdir -p "$(dirname "$SOURCE_DIR")" "$(dirname "$STATE_FILE")"

ensure_secret() {
  name=$1
  value=$2
  if ! docker secret inspect "$name" >/dev/null 2>&1; then
    printf '%s' "$value" | docker secret create "$name" - >/dev/null
  fi
}

sync_source() {
  if [ ! -d "$SOURCE_DIR/.git" ]; then
    git clone --branch "$BRANCH" --single-branch "$REPOSITORY" "$SOURCE_DIR"
  else
    git -C "$SOURCE_DIR" fetch --prune origin "$BRANCH"
    git -C "$SOURCE_DIR" reset --hard FETCH_HEAD
    git -C "$SOURCE_DIR" clean -fdx
  fi
}

deploy() {
  sync_source
  commit=$(git -C "$SOURCE_DIR" rev-parse HEAD)
  ensure_secret noitu_database_url "$NOITU_DATABASE_URL"
  ensure_secret noitu_api_key "$NOITU_API_KEY"
  export NOITU_IMAGE="$IMAGE"
  export ALLOWED_ORIGINS=${ALLOWED_ORIGINS:-https://www.noitu.fun}
  export COLLECT_INTERVAL_MINUTES=${COLLECT_INTERVAL_MINUTES:-30}
  export COLLECT_AUTOSTART=${COLLECT_AUTOSTART:-0}
  export CPU_LIMIT=${CPU_LIMIT:-1.0}
  export MEMORY_LIMIT=${MEMORY_LIMIT:-512M}
  export CPU_RESERVATION=${CPU_RESERVATION:-0.25}
  export MEMORY_RESERVATION=${MEMORY_RESERVATION:-128M}
  docker build -f "$SOURCE_DIR/deployments/Dockerfile" -t "$IMAGE" "$SOURCE_DIR"
  docker stack deploy --resolve-image never -c "$SOURCE_DIR/docker-stack.yml" "$STACK_NAME"
  printf '%s\n' "$commit" >"$STATE_FILE.tmp"
  mv "$STATE_FILE.tmp" "$STATE_FILE"
}

while :; do
  remote=$(git ls-remote "$REPOSITORY" "refs/heads/$BRANCH" | awk 'NR == 1 { print $1 }' || true)
  current=$(cat "$STATE_FILE" 2>/dev/null || true)
  if [ -n "$remote" ] && [ "$remote" != "$current" ]; then
    deploy || echo "deployment failed for $remote; retrying" >&2
  fi
  sleep "$POLL_SECONDS"
done
