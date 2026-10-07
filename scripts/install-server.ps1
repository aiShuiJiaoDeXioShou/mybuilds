# Windows 服务端通过已有 WSL2 + systemd 安装，避免改变其他发行版。
[CmdletBinding()]
param([string]$Distro = 'Ubuntu', [string]$Version = 'v0.1.0', [switch]$WithAgent)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^v\d+\.\d+\.\d+(-[A-Za-z0-9.-]+)?$') { throw '版本格式无效' }
if (-not (Get-Command wsl.exe -ErrorAction SilentlyContinue)) { throw '请先安装 WSL2 发行版并启用 systemd，再重新运行。' }
& wsl.exe -d $Distro -- bash -lc 'test "$(id -u)" -ne 0 && test -d /run/systemd/system && systemctl --user show-environment >/dev/null'
if ($LASTEXITCODE -ne 0) { throw '发行版需要非root默认用户、systemd与可用用户服务；请先按安装文档配置。' }
$extra = if ($WithAgent) { '--with-agent' } else { '' }
$script = 'set -e; f=$(mktemp); trap ''rm -f "$f"'' EXIT; curl --fail --silent --show-error --location --proto ''=https'' https://raw.githubusercontent.com/aiShuiJiaoDeXioShou/mybuilds/' + $Version + '/scripts/install-server.sh -o "$f"; bash "$f" --version ' + $Version + ' ' + $extra
& wsl.exe -d $Distro -- bash -lc $script
if ($LASTEXITCODE -ne 0) { throw 'WSL 内安装失败；已有配置保留。' }
Write-Host "已安装到 $Distro；使用 wsl -d $Distro -- bash -lc 'mybuilds status --json' 检查。"
