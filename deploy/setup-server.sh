#!/usr/bin/env bash
# One-time setup of a fresh Ubuntu 22.04/24.04 VPS. Run as root:
#
#   sudo bash deploy/setup-server.sh
#
# Installs Docker, enables the firewall (SSH + web only), automatic security
# updates and fail2ban, and adds swap on small machines (building the image
# needs ~1 GB of memory). Safe to run again.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root: sudo bash $0" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

echo "==> System packages"
apt-get update -y
apt-get upgrade -y
apt-get install -y ca-certificates curl git ufw unattended-upgrades fail2ban

echo "==> Automatic security updates"
dpkg-reconfigure -f noninteractive unattended-upgrades

echo "==> Docker"
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
fi
systemctl enable --now docker

echo "==> Firewall: only SSH, HTTP, HTTPS (+HTTP/3)"
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp
ufw --force enable

echo "==> Swap (only if RAM < 2 GB and no swap yet)"
ram_mb=$(free -m | awk '/^Mem:/ {print $2}')
if [ "$ram_mb" -lt 2000 ] && ! swapon --show | grep -q .; then
  fallocate -l 2G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
  echo "added 2 GB swap"
fi

echo
echo "Done. Docker $(docker --version | cut -d' ' -f3 | tr -d ,) is ready."
echo "Next: clone the repo, fill deploy/.env, run: bash deploy/update.sh"
