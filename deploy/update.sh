#!/usr/bin/env bash
# Deploys the latest code. Run on the server from anywhere in the repo:
#
#   bash deploy/update.sh
#
# Pulls, rebuilds the image and recreates only the containers that changed.
# The app restarts in ~1-2 s; open clients reconnect on their own and the
# offline queue covers the gap. Migrations run automatically on start.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f deploy/.env ]; then
  echo "deploy/.env is missing: cp deploy/.env.example deploy/.env and fill it in" >&2
  exit 1
fi

git pull --ff-only
export VERSION
VERSION=$(git rev-parse --short HEAD)

docker compose -f deploy/docker-compose.yml up -d --build --wait
docker image prune -f >/dev/null

docker compose -f deploy/docker-compose.yml ps
echo
echo "Deployed version $VERSION:"
docker compose -f deploy/docker-compose.yml exec -T caddy wget -qO- http://app:8080/api/health
echo
