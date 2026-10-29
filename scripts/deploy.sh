#!/bin/sh
set -eu
: "${SLACK_BOT_TOKEN:?missing SLACK_BOT_TOKEN}"
: "${SLACK_APP_TOKEN:?missing SLACK_APP_TOKEN}"

cd "$(dirname "$0")/.."
go build -trimpath -ldflags='-s -w' -o latex-bot .

umask 077
env_dir="$HOME/.config/latex-bot"
mkdir -p "$env_dir"
printf 'SLACK_BOT_TOKEN=%s\nSLACK_APP_TOKEN=%s\n' "$SLACK_BOT_TOKEN" "$SLACK_APP_TOKEN" > "$env_dir/env.tmp"
mv "$env_dir/env.tmp" "$env_dir/env"

systemctl --user enable latex-bot.service >/dev/null 2>&1
systemctl --user restart latex-bot.service
echo "latex-bot deployed and restarted"
