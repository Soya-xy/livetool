# 阿比直播工具

阿比直播工具使用 Wails v3、Go、Vue 3 和 SQLite。桌面端与独立卡密/更新服务分开部署；前端只负责卡密验证、设备安全码和解绑。

## 开发

Requires Go and Node.js 20+.

```sh
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.25 task dev
```

This starts the Wails desktop app and Vite hot reload.

On macOS, the component window's transparent WebView requires the `private_mac_apis` build tag. In GoLand, select the shared run configuration **阿比直播工具 (macOS 透明窗口)**; the temporary auto-generated `go build livetool` configuration omits this tag and shows a white client area. From a terminal, run:

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

Install the Wails v3 beta CLI and set `LIVETOOL_LICENSE_SERVER_URL` to the deployed HTTPS card service. Run `wails3 task windows:package` to produce a Windows amd64 executable and NSIS installer in `bin/`; NSIS (`makensis`) must be installed. The installer sets up the WebView2 Evergreen Runtime when needed, creates Start menu and desktop shortcuts, and installs for the current user. Uninstall removes the program but preserves the user's database and media. Check the installer on a clean Windows machine before distribution. Wails v3 is still beta.

The app keeps settings, logs, its installation ID, and SQLite under the OS user configuration directory, reusing Electron's `阿比整蛊复刻版/data/app.db` location (on Windows: `%APPDATA%/阿比整蛊复刻版/data/app.db`). License sessions stay in memory. Bundled media is copied into the app's assets directory on first launch.

## License and updates

See [backend/README.md](backend/README.md) for the separate card service, operator API, HTTPS reverse-proxy setup, and signed update release steps. The updater uses a Wails Ed25519ph signing key; the private key is ignored by Git and must remain on the release operator's machine.

## Current feature boundary

The migration preserves the original simulator connector, rule engine, local event diary, 18 feature cards, OBS WebSocket control, overlay windows, configuration import/export, and signed hot updates. On Windows amd64, configured keyboard/mouse actions use native `SendInput`, and serial actions enumerate and write to real ports. Live platform ingestion and image search remain unavailable because the Electron source contained only simulator/stub implementations. Native input and serial actions still need acceptance on a Windows machine with the intended target windows and hardware.
