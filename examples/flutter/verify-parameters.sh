#!/bin/bash
set -euo pipefail
# 普通脚本实际消费三个已声明参数；引号使渠道值保持单个数据。
test -n "$APP_VERSION"
printf 'version=%s\nchannel=%s\nflavor=%s\n' "$APP_VERSION" "$APP_CHANNEL" "$APP_FLAVOR"
