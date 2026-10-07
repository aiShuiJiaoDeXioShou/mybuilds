# Windows 原生客户端安装；完整制品下载请使用 WSL 客户端。
[CmdletBinding()]
param(
    [string]$Version = 'v0.1.0',
    [string]$BinDir = (Join-Path $env:LOCALAPPDATA 'mybuilds\bin'),
    [string]$ConfigDir = (Join-Path $env:USERPROFILE '.mybuilds'),
    [string]$SkillsDir = (Join-Path $env:USERPROFILE '.codex\skills'),
    [string]$ServerUrl,
    [string]$TokenFile
)
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw '此入口仅支持 Windows；macOS/Linux 使用 Shell 脚本。' }
if ($Version -notmatch '^v\d+\.\d+\.\d+(-[A-Za-z0-9.-]+)?$') { throw '版本格式无效' }
if ([bool]$ServerUrl -ne [bool]$TokenFile) { throw 'ServerUrl 与 TokenFile 必须一起提供' }
$token = $null
if ($TokenFile) {
    $item = Get-Item -LiteralPath $TokenFile
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'TokenFile 必须是普通文件' }
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $acl = Get-Acl -LiteralPath $TokenFile
    foreach ($rule in $acl.Access) {
        if ($rule.AccessControlType -eq 'Allow') {
            $who = $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
            if ($who -notin @($sid, 'S-1-5-18', 'S-1-5-32-544')) { throw 'TokenFile ACL 允许其他用户读取；请先收紧权限' }
        }
    }
    $token = [IO.File]::ReadAllText($item.FullName).Trim()
    if ($token.Length -lt 32 -or $token.Length -gt 4096 -or $token -match '[^\x21-\x7e]') { throw 'token 格式无效' }
    $url = [Uri]$ServerUrl
    if (-not $url.IsAbsoluteUri -or $url.UserInfo -or $url.Query -or $url.Fragment -or
        ($url.Scheme -ne 'https' -and -not ($url.Scheme -eq 'http' -and $url.IsLoopback))) { throw '服务地址需要 HTTPS 或回环 HTTP' }
}
$archName = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($archName) { 'AMD64' { 'amd64' } 'ARM64' { 'arm64' } default { throw '仅支持64位 amd64/arm64 Windows' } }
$name = "mybuilds_${Version}_windows_${arch}.zip"
$base = "https://github.com/aiShuiJiaoDeXioShou/mybuilds/releases/download/$Version"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp | Out-Null
$installLock = $null
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest "$base/$name" -OutFile (Join-Path $tmp $name) -UseBasicParsing
    Invoke-WebRequest "$base/SHA256SUMS" -OutFile (Join-Path $tmp 'SHA256SUMS') -UseBasicParsing
    $entries = @(Get-Content (Join-Path $tmp 'SHA256SUMS') | Where-Object { ($_ -split '\s+')[1] -eq $name })
    if ($entries.Count -ne 1) { throw '校验清单缺失或重复' }
    $expected = ($entries[0] -split '\s+')[0]
    if ((Get-FileHash (Join-Path $tmp $name) -Algorithm SHA256).Hash -ne $expected) { throw 'SHA-256 校验失败，未安装' }
    $package = Join-Path $tmp 'package'
    Expand-Archive -LiteralPath (Join-Path $tmp $name) -DestinationPath $package
    foreach ($dir in @($BinDir, $ConfigDir, $SkillsDir)) {
        if ((Test-Path -LiteralPath $dir) -and ((Get-Item -LiteralPath $dir).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw '拒绝安装到链接目录' }
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    $lockPath = Join-Path $ConfigDir '.install-lock'
    $installLock = [IO.File]::Open($lockPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    $source = Join-Path $package 'bin\mybuilds.exe'
    $dest = Join-Path $BinDir 'mybuilds.exe'
    if (Test-Path -LiteralPath $dest) {
        if ((Get-Item -LiteralPath $dest).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw '拒绝覆盖链接' }
        if ((Get-FileHash $source).Hash -ne (Get-FileHash $dest).Hash) { throw '已存在不同版本；请备份移走旧程序后重试' }
    } else { [IO.File]::Copy($source, $dest, $false) }
    $config = Join-Path $ConfigDir 'client.yml'
    if ($token -and -not (Test-Path -LiteralPath $config)) {
        $existingToken = [Environment]::GetEnvironmentVariable('MYBUILDS_CLIENT_TOKEN', 'User')
        if ($existingToken -and $existingToken -ne $token) { throw '已有不同用户环境变量token；保留原身份，请显式管理连接' }
        # Windows 当前通过环境变量提供token，避免POSIX 0600配置校验。
        $data = @{ server = $ServerUrl; timeout = '30s' } | ConvertTo-Json
        $stream = [IO.File]::Open($config, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        try {
            $bytes = (New-Object Text.UTF8Encoding($false)).GetBytes($data)
            $stream.Write($bytes, 0, $bytes.Length)
        } finally { $stream.Dispose() }
        [Environment]::SetEnvironmentVariable('MYBUILDS_CLIENT_TOKEN', $token, 'User')
        $env:MYBUILDS_CLIENT_TOKEN = $token
    }
    foreach ($skill in @('mybuilds-deploy', 'mybuilds-operate')) {
        $folder = Join-Path $SkillsDir $skill
        if ((Test-Path -LiteralPath $folder) -and ((Get-Item -LiteralPath $folder).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw '拒绝链接技能目录' }
        New-Item -ItemType Directory -Force -Path $folder | Out-Null
        $src = Join-Path $package "skills\$skill\SKILL.md"
        $dst = Join-Path $folder 'SKILL.md'
        if (Test-Path -LiteralPath $dst) {
            if ((Get-FileHash $src).Hash -ne (Get-FileHash $dst).Hash) { throw "保留已有不同技能文件：$dst" }
        } else { [IO.File]::Copy($src, $dst, $false) }
    }
    $old = [string][Environment]::GetEnvironmentVariable('Path', 'User')
    if (($old -split ';') -notcontains $BinDir) { [Environment]::SetEnvironmentVariable('Path', ($old.TrimEnd(';') + ';' + $BinDir).TrimStart(';'), 'User') }
    if (($env:Path -split ';') -notcontains $BinDir) { $env:Path = "$BinDir;$env:Path" }
    & $dest version
    if ($LASTEXITCODE -ne 0) { throw '程序验证失败' }
    Write-Host "安装完成：$BinDir。新终端可用 mybuilds；导入的 token 保存在用户环境变量，新终端生效。"
    Write-Host 'Windows 原生支持远程管理；artifact download 尚不支持，请用 WSL。'
} finally {
    $token = $null
    if ($installLock) { $installLock.Dispose(); Remove-Item -LiteralPath $lockPath }
    Remove-Item -LiteralPath $tmp -Recurse -Force
}
