#!/usr/bin/env bash
# 下载固定版本并校验；其余参数原样传给包内安装器。
set -euo pipefail
version=v0.1.0
if [[ ${1:-} == --version ]]; then
  [[ $# -ge 2 ]] || { echo '--version 缺少值' >&2; exit 1; }
  version=$2; shift 2
fi
if [[ ${1:-} == --help ]]; then
  echo '用法：install-client.sh [--version vX.Y.Z] [安装选项]'
  echo '通用：--bin-dir PATH --config-dir PATH --skills-dir PATH --no-path'
  echo '客户端：--server-url URL --token-file PATH [--ca-file PATH]'
  echo '服务端：--with-agent --service user|system|none --port 8787'
  exit 0
fi
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || { echo '版本格式无效' >&2; exit 1; }
case "$(uname -s)" in Darwin) os=darwin;; Linux) os=linux;; *) echo '此脚本仅支持 macOS/Linux' >&2; exit 1;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64;; x86_64|amd64) arch=amd64;; *) echo '仅支持 amd64/arm64' >&2; exit 1;; esac
for tool in curl tar python3; do command -v "$tool" >/dev/null || { echo "需要 $tool" >&2; exit 1; }; done
archive="mybuilds_${version}_${os}_${arch}.tar.gz"
base="https://github.com/aiShuiJiaoDeXioShou/mybuilds/releases/download/$version"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$base/$archive" -o "$tmp/$archive"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
python3 - "$tmp" "$archive" <<'PYTHON'
import hashlib,pathlib,sys,tarfile
folder=pathlib.Path(sys.argv[1]); name=sys.argv[2]
lines=[line.split() for line in (folder/'SHA256SUMS').read_text().splitlines()]
expected=[parts[0] for parts in lines if len(parts)==2 and parts[1]==name]
if len(expected)!=1 or hashlib.sha256((folder/name).read_bytes()).hexdigest()!=expected[0]:
    sys.exit('SHA-256 校验失败，未安装')
with tarfile.open(folder/name) as archive:
    for member in archive.getmembers():
        p=pathlib.PurePosixPath(member.name)
        if p.is_absolute() or '..' in p.parts or not (member.isfile() or member.isdir()):
            sys.exit('发行包包含非法路径或链接，未安装')
    archive.extractall(folder/'package')
PYTHON
bash "$tmp/package/scripts/install.sh" client "$@"
