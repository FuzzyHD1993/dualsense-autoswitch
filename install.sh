#!/usr/bin/env bash
# Установка dualsense-autoswitch: бинарь + user unit + пример конфига.
set -euo pipefail
cd "$(dirname "$0")"

command -v go >/dev/null || { echo "нужен Go 1.21+"; exit 1; }

echo "==> сборка"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dualsense-autoswitch .
install -Dm755 dualsense-autoswitch "$HOME/.local/bin/dualsense-autoswitch"

echo "==> конфиг"
mkdir -p "$HOME/.config/dualsense"
[ -f "$HOME/.config/dualsense/profiles.conf" ] || \
    install -m644 examples/profiles.conf "$HOME/.config/dualsense/profiles.conf"
touch "$HOME/.config/dualsense/override"

echo "==> профили"
[ -f "$HOME/.local/bin/dualsense-variant" ] || \
    install -m755 profiles/dualsense-variant "$HOME/.local/bin/dualsense-variant"
for p in aw2 rdr2 poe2 cd ets2 cp2077 default; do
    ln -sf dualsense-variant "$HOME/.local/bin/dualsense-profile-$p"
done

echo "==> systemd"
install -Dm644 systemd/dualsense-autoswitch.service "$HOME/.config/systemd/user/dualsense-autoswitch.service"
systemctl --user daemon-reload
systemctl --user enable --now dualsense-autoswitch.service

echo "==> готово"
echo "конфиг: $HOME/.config/dualsense/profiles.conf"
echo "проверка: dualsense-autoswitch -once"
