[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$ReleaseDir)
$ErrorActionPreference = 'Stop'
$ReleaseDir = (Resolve-Path $ReleaseDir).Path
# 用本次真实构建包替代网络下载，其余安装流程不模拟。
function Invoke-WebRequest {
    param([string]$Uri, [string]$OutFile, [switch]$UseBasicParsing)
    $name = ([Uri]$Uri).Segments[-1]
    Copy-Item -LiteralPath (Join-Path $ReleaseDir $name) -Destination $OutFile
}
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp | Out-Null
$oldPath = [Environment]::GetEnvironmentVariable('Path', 'User')
try {
    foreach ($script in @('install-client.ps1', 'install-server.ps1')) {
        $tokens = $null; $errors = $null
        [Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $script), [ref]$tokens, [ref]$errors) | Out-Null
        if ($errors.Count) { throw ($errors | Out-String) }
    }
    $options = @{ BinDir = (Join-Path $tmp 'with space\bin'); ConfigDir = (Join-Path $tmp 'config'); SkillsDir = (Join-Path $tmp 'skills') }
    & (Join-Path $PSScriptRoot 'install-client.ps1') @options
    & (Join-Path $options.BinDir 'mybuilds.exe') --help | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'CLI运行失败' }
    & (Join-Path $PSScriptRoot 'install-client.ps1') @options
    $exe = Join-Path $options.BinDir 'mybuilds.exe'
    [IO.File]::WriteAllText($exe, 'preserve me')
    $failed = $false
    try { & (Join-Path $PSScriptRoot 'install-client.ps1') @options } catch { $failed = $true }
    if (-not $failed -or [IO.File]::ReadAllText($exe) -ne 'preserve me') { throw '已有文件保护失败' }
    Write-Host 'PASS Windows PowerShell语法、真实二进制、首次/重复安装与已有文件保护'
} finally {
    [Environment]::SetEnvironmentVariable('Path', $oldPath, 'User')
    Remove-Item -LiteralPath $tmp -Recurse -Force
}
