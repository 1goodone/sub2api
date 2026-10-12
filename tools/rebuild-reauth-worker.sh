#!/bin/sh
# Rebuild the OpenAI reauth worker from this repo and roll both instances.
#
# The image Dockerfile lives at tools/Dockerfile.openai-oauth-reauth-worker.
# Turb and toSub2 are pinned build contexts, not mounts and not files copied
# into a running container:
#   TURB_CONTEXT   default /home/ubuntu/src/reauth-contexts/turb
#   TOSUB2_CONTEXT default /home/ubuntu/src/reauth-contexts/tosub2
# Those trees were copied from
# sub2api-openai-reauth-worker:7b7e30923-server40-20261003-r1, the runtime the
# Excel adapter matches. Replace a context only after tools/rebuild-reauth-worker.sh
# check still passes.
#
# Usage:
#   tools/rebuild-reauth-worker.sh check
#   tools/rebuild-reauth-worker.sh build <image-tag>
#   tools/rebuild-reauth-worker.sh roll <image-tag> <ai-container> <mr-container>
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TURB=${TURB_CONTEXT:-/home/ubuntu/src/reauth-contexts/turb}
TOSUB2=${TOSUB2_CONTEXT:-/home/ubuntu/src/reauth-contexts/tosub2}
AI_ENV=${AI_WORKER_ENV:-/root/sub2api-reauth-worker.env}
MR_ENV=${MR_WORKER_ENV:-/root/sub2api-reauth-worker-mr.env}
AI_NAME=${AI_WORKER_NAME:-sub2api-openai-reauth-worker}
MR_NAME=${MR_WORKER_NAME:-sub2api-openai-reauth-worker-mr}

require_contexts() {
  test -f "$TURB/LICENSE"
  test -f "$TURB/core/codex_oauth.py"
  test -d "$TURB/sentinel"
  test -f "$TOSUB2/LICENSE"
  test -f "$TOSUB2/src/protocol-login.mjs"
}

check_excel() {
  require_contexts
  PYTHONPATH="$ROOT/tools${PYTHONPATH:+:$PYTHONPATH}" python3 - "$TOSUB2/src/protocol-login.mjs" <<'PY'
import sys
from pathlib import Path
from openai_excel_oauth_adapter import excel_source
excel_source(Path(sys.argv[1]).read_text())
print("excel adapter matches pinned toSub2")
PY
}

build_image() {
  tag=$1
  require_contexts
  check_excel
  sudo docker build \
    --build-context "turb=$TURB" \
    --build-context "tosub2=$TOSUB2" \
    -f "$ROOT/tools/Dockerfile.openai-oauth-reauth-worker" \
    -t "$tag" \
    "$ROOT"
}

roll_one() {
  tag=$1
  name=$2
  app=$3
  envfile=$4
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  if sudo docker inspect "$name" >/dev/null 2>&1; then
    sudo docker rename "$name" "${name}-prev-${stamp}"
    sudo docker stop "${name}-prev-${stamp}" >/dev/null
  fi
  sudo docker run -d --name "$name" --restart unless-stopped \
    --network "container:${app}" \
    --env-file "$envfile" \
    "$tag" >/dev/null
}

roll_workers() {
  tag=$1
  ai_app=$2
  mr_app=$3
  roll_one "$tag" "$AI_NAME" "$ai_app" "$AI_ENV"
  roll_one "$tag" "$MR_NAME" "$mr_app" "$MR_ENV"
  for name in "$AI_NAME" "$MR_NAME"; do
    sudo docker exec "$name" python -c "import openai_excel_oauth_adapter, openai_excel_2fa_login"
  done
  sleep 2
  sudo docker inspect -f '{{.Name}} {{.Config.Image}} {{.HostConfig.NetworkMode}}' "$AI_NAME" "$MR_NAME"
  sudo docker logs --tail 4 "$AI_NAME"
  sudo docker logs --tail 4 "$MR_NAME"
}

command=${1:-}
case "$command" in
  check)
    check_excel
    ;;
  build)
    test $# -eq 2
    build_image "$2"
    ;;
  roll)
    test $# -eq 4
    roll_workers "$2" "$3" "$4"
    ;;
  *)
    echo "usage: tools/rebuild-reauth-worker.sh check | build <image-tag> | roll <image-tag> <ai-container> <mr-container>" >&2
    exit 2
    ;;
esac
