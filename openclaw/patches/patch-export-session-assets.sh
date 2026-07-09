#!/usr/bin/env bash
set -euo pipefail

ln -sfn auto-reply/reply/export-html /usr/lib/node_modules/openclaw/dist/export-html
test -e /usr/lib/node_modules/openclaw/dist/export-html/template.html
