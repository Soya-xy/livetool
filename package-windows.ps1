<#
.SYNOPSIS
  把 livetool（阿比直播工具）打包成 Windows 可执行文件，可选产出 zip 与 NSIS 安装包。

.DESCRIPTION
  纯 PowerShell 实现，不依赖 task / bash，可直接在 Windows 上运行。
  步骤：go test → 生成 Wails 绑定 → 构建前端 → 生成 Windows 资源(.syso：图标/清单/版本号)
        → go build（production 标签 + windowsgui）→ 输出 bin\livetool.exe
        → 可选打 zip（dist\）→ 可选生成 NSIS 安装包（需要 makensis）。
  与 Taskfile.yml 的 windows:build / windows:package 使用同一套参数，产物路径一致。

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\package-windows.ps1

.EXAMPLE
  # 指定版本号与授权服务地址，并顺便出安装包
  powershell -ExecutionPolicy Bypass -File .\package-windows.ps1 -Version 0.3.0 -LicenseServerUrl https://license.example.com -Installer

.EXAMPLE
  # 只想快速重编 exe（跳过绑定 / 前端 / 资源）
  powershell -ExecutionPolicy Bypass -File .\package-windows.ps1 -SkipBindings -SkipFrontend -SkipSyso
#>
[CmdletBinding()]
param(
  # 版本号；留空则读 main.go 里的 var Version
  [string]$Version = '',
  # 卡密授权服务地址（必须是 https）；也可用环境变量 LIVETOOL_LICENSE_SERVER_URL
  [string]$LicenseServerUrl = $env:LIVETOOL_LICENSE_SERVER_URL,
  # 目标架构
  [ValidateSet('amd64', 'arm64')][string]$Arch = 'amd64',
  # 跳过 Wails TypeScript 绑定生成
  [switch]$SkipBindings,
  # 跳过前端构建（前端产物会被嵌进 exe）
  [switch]$SkipFrontend,
  # 跳过 Windows 资源(.syso)生成，exe 将没有自定义图标与版本信息
  [switch]$SkipSyso,
  # 不生成 zip
  [switch]$NoZip,
  # 额外生成 NSIS 安装包（需要 makensis）
  [switch]$Installer,
  # 跳过 go test
  [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Set-Location -LiteralPath $PSScriptRoot

$WAILS_VERSION = 'v3.0.0-beta.25'
$APP_NAME = 'livetool'
$BinDir = Join-Path $PSScriptRoot 'bin'
$DistDir = Join-Path $PSScriptRoot 'dist'
$ExePath = Join-Path $BinDir "$APP_NAME.exe"
$StartTime = Get-Date

function Write-Step([string]$Text) { Write-Host ""; Write-Host "[$(Get-Date -Format 'HH:mm:ss')] $Text" -ForegroundColor Cyan }
function Write-Ok([string]$Text) { Write-Host "      $Text" -ForegroundColor Green }
function Write-Warn2([string]$Text) { Write-Host "      $Text" -ForegroundColor Yellow }
function Fail([string]$Text) { Write-Host ""; Write-Host "[错误] $Text" -ForegroundColor Red; exit 1 }

function Invoke-Checked([string]$Label, [scriptblock]$Action) {
  & $Action
  if ($LASTEXITCODE -ne 0) { Fail "$Label 失败（退出码 $LASTEXITCODE）。" }
}

# ── 0. 环境：Go / Node ────────────────────────────────────────────────────
Write-Step '检查构建环境'

function Resolve-Go {
  if (Get-Command go -ErrorAction SilentlyContinue) { return }
  foreach ($dir in @("$env:ProgramFiles\Go\bin", "$env:LOCALAPPDATA\Programs\Go\bin", 'C:\Go\bin')) {
    if (Test-Path (Join-Path $dir 'go.exe')) { $env:PATH = "$dir;$env:PATH"; return }
  }
  Fail '找不到 Go。请先安装：winget install GoLang.Go'
}

function Resolve-Node {
  if (Get-Command node -ErrorAction SilentlyContinue) { return }
  $candidates = @(
    "$env:ProgramFiles\nodejs",
    "${env:ProgramFiles(x86)}\nodejs",
    "$env:LOCALAPPDATA\Programs\nodejs",
    "$env:APPDATA\fnm\aliases\default",
    "$env:APPDATA\nvm"
  )
  foreach ($dir in $candidates) {
    if ($dir -and (Test-Path (Join-Path $dir 'node.exe'))) { $env:PATH = "$dir;$env:PATH"; return }
  }
  foreach ($root in @("$env:APPDATA\fnm\node-versions", "$env:LOCALAPPDATA\fnm\node-versions")) {
    if (Test-Path $root) {
      $newest = Get-ChildItem $root -Directory | Sort-Object Name -Descending | Select-Object -First 1
      if ($newest -and (Test-Path (Join-Path $newest.FullName 'installation\node.exe'))) {
        $env:PATH = "$(Join-Path $newest.FullName 'installation');$env:PATH"
        return
      }
    }
  }
  Fail '找不到 Node.js（需要 20 以上）。请安装：winget install OpenJS.NodeJS.LTS，或先用 fnm use 指定版本。'
}

Resolve-Go
Resolve-Node
Write-Ok "Go     $(go version)"
Write-Ok "Node   $(node -v)"

# ── 版本号 ────────────────────────────────────────────────────────────────
if (-not $Version) {
  $mainGo = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'main.go') -Raw
  if ($mainGo -match 'var\s+Version\s*=\s*"([^"]+)"') { $Version = $Matches[1] } else { $Version = '0.0.0' }
}
Write-Ok "版本   $Version"

# ── 授权服务地址 ──────────────────────────────────────────────────────────
if ($null -ne $LicenseServerUrl) { $LicenseServerUrl = $LicenseServerUrl.Trim() } else { $LicenseServerUrl = '' }
if (-not $LicenseServerUrl) {
  Write-Warn2 '未提供 -LicenseServerUrl，exe 内不含授权服务地址：'
  Write-Warn2 '  发布版本请加 -LicenseServerUrl https://你的授权服务，否则用户无法在线激活。'
} elseif ($LicenseServerUrl -notmatch '^https://') {
  Fail "LicenseServerUrl 必须是 https 地址，当前为：$LicenseServerUrl"
}

# ── 1. 自检：go test ──────────────────────────────────────────────────────
if (-not $SkipTests) {
  Write-Step '运行 go test ./...'
  Invoke-Checked 'go test' { go test ./... }
  Write-Ok '测试通过'
} else {
  Write-Warn2 '已跳过测试（-SkipTests）'
}

# ── 2. Wails 绑定 ─────────────────────────────────────────────────────────
if (-not $SkipBindings) {
  Write-Step '生成 Wails TypeScript 绑定'
  Invoke-Checked 'wails3 generate bindings' {
    go run "github.com/wailsapp/wails/v3/cmd/wails3@$WAILS_VERSION" generate bindings -ts -i -d frontend/bindings
  }
  Write-Ok 'frontend/bindings 已更新'
} else {
  Write-Warn2 '已跳过绑定生成（-SkipBindings）'
}

# ── 3. 前端构建 ───────────────────────────────────────────────────────────
if (-not $SkipFrontend) {
  if (-not (Test-Path (Join-Path $PSScriptRoot 'frontend\node_modules'))) {
    Write-Step '安装前端依赖（npm ci）'
    Invoke-Checked 'npm ci' { npm --prefix frontend ci }
  }
  Write-Step '构建前端（npm run build）'
  Invoke-Checked 'npm run build' { npm --prefix frontend run build }
  Write-Ok 'frontend/dist 已生成'
} else {
  Write-Warn2 '已跳过前端构建（-SkipFrontend）：exe 会用 frontend/dist 里现有的产物'
  if (-not (Test-Path (Join-Path $PSScriptRoot 'frontend\dist\index.html'))) {
    Fail 'frontend/dist 不存在，无法跳过前端构建。'
  }
}

# ── 4. Windows 资源（图标 / 清单 / 版本信息）───────────────────────────────
if (-not $SkipSyso) {
  Write-Step "生成 Windows 资源 wails_windows_$Arch.syso"
  $sysoOk = $true
  try {
    Invoke-Checked 'wails3 generate syso' {
      go run "github.com/wailsapp/wails/v3/cmd/wails3@$WAILS_VERSION" generate syso -arch $Arch -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out "wails_windows_$Arch.syso"
    }
  } catch {
    $sysoOk = $false
  }
  if ($sysoOk -and (Test-Path (Join-Path $PSScriptRoot "wails_windows_$Arch.syso"))) {
    Write-Ok '资源文件已生成'
  } else {
    Write-Warn2 '资源生成失败（常见原因：离线 / 缺少 wails3 依赖），本次继续，exe 将使用默认图标。'
  }
} else {
  Write-Warn2 '已跳过资源生成（-SkipSyso）'
}

# ── 5. 编译 exe ───────────────────────────────────────────────────────────
Write-Step "编译 Windows $Arch 可执行文件"
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
$ldflags = "-s -w -H=windowsgui -X main.Version=$Version"
if ($LicenseServerUrl) { $ldflags += " -X main.LicenseServerURL=$LicenseServerUrl" }
$env:GOOS = 'windows'
$env:GOARCH = $Arch
$env:CGO_ENABLED = '0'
Invoke-Checked 'go build' { go build -tags production -trimpath -buildvcs=false -ldflags $ldflags -o $ExePath . }
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
if (-not (Test-Path $ExePath)) { Fail "没有生成 $ExePath。" }
$exe = Get-Item $ExePath
$hash = (Get-FileHash -LiteralPath $ExePath -Algorithm SHA256).Hash
Write-Ok ("{0}  {1:N1} MB" -f $exe.FullName, ($exe.Length / 1MB))
Write-Ok "SHA256  $hash"

# ── 6. 打 zip ─────────────────────────────────────────────────────────────
if (-not $NoZip) {
  Write-Step '打包 zip'
  New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
  $readme = Join-Path $DistDir '使用说明.txt'
  $lines = @(
    "阿比直播工具 $Version（Windows $Arch）",
    "",
    "1. 解压后直接运行 livetool.exe，无需安装。",
    "2. 首次运行会把内置素材释放到 %APPDATA%\AKA直播复刻版\assets，数据库与日志也在该目录下。",
    "3. 运行环境：Windows 10/11，需要 WebView2 运行时（Win11 自带；Win10 若提示缺失，安装 MicrosoftEdgeWebview2Setup.exe 即可）。",
    "4. 卸载：直接删除本目录，用户数据不会被删除。"
  )
  Set-Content -LiteralPath $readme -Value $lines -Encoding UTF8
  $zip = Join-Path $DistDir "$APP_NAME-$Version-windows-$Arch.zip"
  Compress-Archive -LiteralPath $ExePath, $readme -DestinationPath $zip -Force
  Write-Ok ("{0}  {1:N1} MB" -f $zip, ((Get-Item $zip).Length / 1MB))
}

# ── 7. NSIS 安装包（可选）─────────────────────────────────────────────────
if ($Installer) {
  Write-Step '生成 NSIS 安装包'
  $makensis = Get-Command makensis -ErrorAction SilentlyContinue
  if (-not $makensis) {
    Write-Warn2 '没找到 makensis，跳过安装包。安装方式：winget install NSIS.NSIS'
  } else {
    Invoke-Checked 'wails3 generate webview2bootstrapper' {
      go run "github.com/wailsapp/wails/v3/cmd/wails3@$WAILS_VERSION" generate webview2bootstrapper --dir build/windows/nsis
    }
    Push-Location (Join-Path $PSScriptRoot 'build\windows\nsis')
    try {
      Invoke-Checked 'makensis' {
        makensis -DINFO_PRODUCTVERSION=$Version "-DARG_WAILS_AMD64_BINARY=..\..\..\bin\$APP_NAME.exe" project.nsi
      }
    } finally {
      Pop-Location
    }
    $installer = Join-Path $BinDir "$APP_NAME-$Version-$Arch-installer.exe"
    if (Test-Path $installer) { Write-Ok ("{0}  {1:N1} MB" -f $installer, ((Get-Item $installer).Length / 1MB)) }
  }
}

# ── 完成 ──────────────────────────────────────────────────────────────────
$elapsed = [int]((Get-Date) - $StartTime).TotalSeconds
Write-Host ""
Write-Host "打包完成（$elapsed 秒）：" -ForegroundColor Green
Write-Host "  可执行文件  $ExePath"
if (-not $NoZip) { Write-Host "  压缩包      $(Join-Path $DistDir "$APP_NAME-$Version-windows-$Arch.zip")" }
if ($Installer) {
  $installerPath = Join-Path $BinDir "$APP_NAME-$Version-$Arch-installer.exe"
  if (Test-Path $installerPath) { Write-Host "  安装包      $installerPath" }
}
Write-Host ""
