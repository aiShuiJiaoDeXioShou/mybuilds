#!/usr/bin/env bash
set -euo pipefail
command -v python3 >/dev/null || { echo '需要 Python 3，请先安装。' >&2; exit 1; }
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$script_dir/install.py" "$@"
