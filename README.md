# AKA直播

AKA直播使用 Wails v3、Go、Vue 3 和 SQLite。桌面端与独立卡密/更新服务分开部署；前端只负责卡密验证、设备安全码和解绑。

## 开发

Requires Go and Node.js 20+.

```sh
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.25 task dev
```

This starts the Wails desktop app and Vite hot reload.

On macOS, the component window's transparent WebView requires the `private_mac_apis` build tag. In GoLand, select the shared run configuration **AKA直播 (macOS 透明窗口)**; the temporary auto-generated `go build livetool` configuration omits this tag and shows a white client area. From a terminal, run:

```sh
go run -tags private_mac_apis .
```

The native titlebar remains visible and opaque. The component window client area is transparent, while each component card keeps its own opaque background.

To run only frontend type/build checks:

```sh
npm --prefix frontend ci
npm --prefix frontend run check
npm --prefix frontend run build
```

The production Wails binary embeds `frontend/dist` and `resources/`. Generate bindings before standalone frontend checks with:

```sh
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.25 generate bindings -ts -i -d frontend/bindings
```

## Desktop build

Windows 上直接用仓库根目录的 PowerShell 脚本即可（不需要 task / bash / Git Bash）：

```powershell
# 完整打包：go test → 生成绑定 → 构建前端 → 生成图标与版本资源 → 编译 exe → 打 zip
powershell -ExecutionPolicy Bypass -File .\package-windows.ps1

# 发布版本：带上版本号与授权服务地址，并额外生成 NSIS 安装包
powershell -ExecutionPolicy Bypass -File .\package-windows.ps1 -Version 0.3.0 -LicenseServerUrl https://license.example.com -Installer
```

产物：`bin/livetool.exe`（自包含，内置前端与素材）、`dist/livetool-<版本>-windows-<架构>.zip`（exe + 使用说明），
加 `-Installer` 时再有 `bin/livetool-<版本>-<架构>-installer.exe`（需要 NSIS：`winget install NSIS.NSIS`）。
可用的开关：`-SkipTests`、`-SkipBindings`、`-SkipFrontend`、`-SkipSyso`、`-NoZip`、`-Arch arm64`。

If the Wails v3 CLI is installed, `wails3 task windows:package` does the same thing from the Taskfile (it needs a POSIX `sh` for its preconditions, so on Windows the PowerShell script above is usually easier). Set `LIVETOOL_LICENSE_SERVER_URL` to the deployed HTTPS card service before a release build; the installer sets up the WebView2 Evergreen Runtime when needed, creates Start menu and desktop shortcuts, and installs for the current user. Uninstall removes the program but preserves the user's database and media. Check the installer on a clean Windows machine before distribution. Wails v3 is still beta.

The app keeps settings, logs, its installation ID, and SQLite under the OS user configuration directory, reusing Electron's `AKA直播复刻版/data/app.db` location (on Windows: `%APPDATA%/AKA直播复刻版/data/app.db`). License sessions stay in memory. Bundled media is copied into the app's assets directory on first launch.

## License and updates

See [backend/README.md](backend/README.md) for the separate card service, operator API, HTTPS reverse-proxy setup, and signed update release steps. The updater uses a Wails Ed25519ph signing key; the private key is ignored by Git and must remain on the release operator's machine.

## Current feature boundary

The migration preserves the original simulator connector, rule engine, local event diary, 18 feature cards, OBS WebSocket control, overlay windows, configuration import/export, and signed hot updates. On Windows amd64, configured keyboard/mouse actions use native `SendInput`, and serial actions enumerate and write to real ports. 直播平台接入现已实现抖音（`im/fetch` 拉取线路，无需签名，见 [docs/直播平台连接器实现.md](docs/直播平台连接器实现.md)）；快手、视频号、B站、TIKTOK、斗鱼、小红书仍返回「连接器尚未实现」，image search 也仍不可用。Native input and serial actions still need acceptance on a Windows machine with the intended target windows and hardware.
