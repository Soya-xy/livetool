# 阿比整蛊复刻版 · Electron → Wails 迁移方案

> 依据：当前仓库已实现的 Electron 版（`livetool/`）+ 《1比1复刻实现总设计文档-Wails版.md》

## 当前迁移落地状态（2026-09-24）

- 桌面端已切换为 Wails v3 + Go 服务，Vue 3 前端改用生成的 Wails 绑定；Electron 主进程、预加载层、Electron Vite 配置和 npm 根依赖不再作为运行入口。
- SQLite、规则/动作执行、18 个功能配置、模拟器事件、弹幕记录、OBS WebSocket、配置导入导出、日志和诊断已移入 Go/Wails 服务。直播平台连接保持原版本地模拟器；未实现的平台现在明确拒绝连接，不再误报为已连接。本轮确认原 Electron 连接器本身未实现；Windows amd64 键鼠操作接入真实 `SendInput`，串口操作接入真实端口，查图沿用原版未执行响应。本地开发授权只在非 production 构建中开放。
- 事件输入通过 1000 条上限的 FIFO 队列，由 4 个消费者并发处理；同类重复事件在 3 秒内去重，队列满时优先保留礼物，排队超过 30 秒则丢弃并累计到 `ConnectorStatus.dropped`。
- 用户数据目录沿用 Electron `productName`（`阿比整蛊复刻版`），继续使用 `%APPDATA%/阿比整蛊复刻版/data/app.db`，不另开 Wails 专用数据库目录。
- 卡密校验和签名更新 API 独立放在 `backend/`；桌面前端没有管理员页面，只提供卡密校验、安全码和本机解绑。
- Wails 更新器已使用嵌入公钥验证发布清单和产物；授权后检查，安装和重启前会询问用户。后端通过 `UPDATE_MANIFEST_FILE` 与 `UPDATE_ARTIFACT_DIR` 提供清单和二进制。
- 生产构建会把旧设置或导入配置中的 `devMode` 强制归零，且规则调试快捷键增加 production build tag 门禁，避免开发配置遗留后绕开授权入口触发动作。
- 主窗口和常用卡片布局采用固定浅色主题：白色卡片、浅冷灰工作区、低饱和靛蓝主色，不提供暗黑模式。控制中心合并常用操作并压缩搜索区；卡密、模拟器、功能设置和规则编辑弹窗分别收窄至 380、460、560 和 700px，长表单在内部滚动。叠加窗保留原生标题栏并禁用最小化/最大化；本地媒体由同源资源路由提供并限制在素材目录内。x64 程序已在 Windows 11 ARM64 虚拟机中通过兼容层启动并检查主界面和卡密弹窗；原生 Windows amd64 窗口、OBS 采集、更新安装和真实设备验收仍待目标机完成。

前端 Vue 类型检查、Vite 生产构建、Wails macOS 生产构建、Windows x64 交叉编译和卡密服务 API 冒烟结果记录在 `docs/implementation-evidence.md`。这些检查不代表 Windows 原生窗口、OBS、物理设备或更新替换已在目标机运行通过。
> 目标：把桌面壳从 Electron 换成 **Wails v3（Go + WebView2）**，前端 Vue 3 与数据/规则逻辑尽量原样搬运，并拿到 Electron 拿不到的能力——**系统原生标题栏 + 窗口级真透明**。

---

## 0. 为什么要转（结论先行）

### 0.1 原版 yapp.exe 的做法（实测）

| 观测项 | 实测结果 |
|---|---|
| 技术栈 | Wails（Go + WebView2），符号表含 `main.SetLayeredWindowAttributes`、`main.(*App).touming/quxiaotouming`、`main.regHotkey` |
| 窗口样式 | `WS_CAPTION=True`（系统原生标题栏）、`WS_THICKFRAME=False` |
| 客户区底色 | 纯黑（PrintWindow 抓图：原生标题栏 + 一整块黑） |
| 透明机制 | Ctrl+F1 → 给窗口加 `WS_EX_LAYERED`，再 `SetLayeredWindowAttributes(hwnd, 0x000000, 0, LWA_COLORKEY)` 把黑底抠成透明；组件与系统标题栏不受影响（符号 + 黑底 + 运行中的透明态截图三方印证） |
| 能成立的原因 | WebView2 是**子窗口**，它自己不画底；被抠掉的是父窗口（GDI 合成）的黑底 |

> 证据等级：窗口样式/底色为**实测**；抠像参数由 Go 符号表 + 默认黑底 + 透明态表现**推断**（原版热键在我这台机器上被占用，未直接抓到切换瞬间）。

### 0.2 Electron 为什么复刻不了（三条路均已实测失败）

| 尝试 | 结果 |
|---|---|
| `transparent: true` | 系统不给标题栏（实测 `WS_CAPTION=False`） |
| 透明窗口运行时补 `WS_CAPTION` | Chromium 逐像素透明失效，客户区变白 |
| 普通窗口 + `WS_EX_LAYERED` + 黑色 `LWA_COLORKEY` | 属性生效（`key=#000000 flags=1`）但 Chromium 内容走 DirectComposition 合成，颜色键对内容无效，黑底依旧不透明 |

结论：Electron 的透明窗口路径无法同时满足原生标题栏和透明客户区，因此迁移到 Wails。Wails 组件窗使用 `BackgroundTypeTransparent` 与 alpha 0 的 WebView2 背景；窗口保持普通带框样式（`Frameless: false`），系统标题栏继续由 Windows 绘制。不要再用 `WS_EX_LAYERED` 黑色色键处理 Chromium 客户区。

---

## 1. 现状盘点（Electron 版已实现什么）

### 1.1 主进程（`src/main/`，约 2 000 行）

| 文件 | 行数 | 职责 | 迁移方式 |
|---|---:|---|---|
| `index.ts` | 896 | 63 个 IPC 通道、设置读写、日志、授权、全局热键、动作派发、配置导入导出 | 拆成 Go 服务对象（见 §3.2） |
| `services/windows.ts` | 296 | 主窗 + 绿幕窗 + 组件窗 + 音频隐藏窗；模式切换、底板、消息下发 | 重写为 Go 窗口管理器（见 §3.1） |
| `services/store.ts` | 222 | sql.js（WASM SQLite）：`rules` / `danmaku_records` / `logs` / `settings` 四张表 | 换 `modernc.org/sqlite`，DDL 照搬 |
| `services/license-client.ts` | 208 | 自建授权服务客户端：登录/状态/登出/安全码/解绑 + 卡密后台 | Go HTTP 客户端，协议不变 |
| `core/rule-engine.ts` | 125 | `matchRule` + 冷却/队列调度 | 1:1 移植为 Go package |
| `services/obs-websocket.ts` | 136 | OBS WebSocket v5 客户端（含鉴权、虚拟摄像头、滤镜） | Go 客户端，逻辑照搬 |
| `services/connectors.ts` | 73 | 连接器管理 + 本地模拟器事件源 | Go package，接口不变 |
| `services/logger.ts` | 43 | 分级日志 + 落库 + 推送 | Go package |
| `services/action-executor.ts` | 38 | 8 类动作执行入口 | Go package |

### 1.2 预加载层

- `src/preload/index.ts`：`contextBridge` 暴露 `window.api`，形状即 `ElectronApi`（`src/shared/types.ts`）。
- **迁移时整个删掉**，由 Wails 生成的绑定 + 一层前端适配器替代（见 §3.7）。

### 1.3 渲染层（`src/renderer/`，约 2 400 行，可原样复用）

| 文件 | 行数 | 说明 |
|---|---:|---|
| `App.vue` | 170 | 壳：侧栏、标题栏、授权弹窗、页面错误边界 |
| `pages/ControlCenter.vue` | 68 | 规则列表、模拟事件、导入导出 |
| `pages/DanmakuDiary.vue` | 154 | 实时弹幕 + 历史检索/导出/清理 |
| `pages/Extensions.vue` | 273 | 18 个功能开关、窗口工具条、功能设置入口 |
| `pages/Settings.vue` | 102 | 平台连接、Overlay、弹幕记录、OBS、诊断 |
| `pages/LicenseAdmin.vue` | 216 | 卡密后台 |
| `pages/OverlayGreen.vue` | 116 | 绿幕：视频、砸落物、状态条 |
| `pages/OverlaySlot.vue` | 434 | 组件窗：12 类组件、水果机、底板两态 |
| `pages/AudioOverlay.vue` | 27 | 隐藏音频窗 |
| `components/RuleEditorDialog.vue` | 90 | 规则编辑器 |
| `components/FeatureSettingsDialog.vue` | 254 | 功能参数编辑器 |
| `services/api.ts` | 138 | IPC 适配层（浏览器兜底 API） |
| `stores/app.ts` | 42 | Pinia：规则、实时弹幕、状态、设置 |
| `styles.css` | 246 | 全部视觉样式 |

### 1.4 共享层与其它

| 文件 | 说明 |
|---|---|
| `src/shared/types.ts` | 全部数据模型：`LiveEvent`、`Rule`、`Action`、`DanmakuRecord`、`AppSettings`、`OverlaySettings/Status`、`License*`、`ElectronApi` |
| `src/shared/features.ts` | 18 个功能定义（`component-window`、`slot-machine`、`electronic-woodfish`、`mosquito-slap`、`screen-lock`、`speed-curve`、`voice-broadcast` …）与默认参数 |
| `server/index.mjs` + `server/README.md` | 自建卡密授权服务（Node），**迁移后保持独立部署，不动** |
| `scripts/self-check.ts` | 规则引擎自检（匹配/正则/数量/冷却/动作执行）→ 迁移后对应 Go 单测 |
| 数据文件 | `app.getPath('userData')/app.db`（SQLite）、`resources/` 素材根 |

### 1.5 现有协议与通道（迁移后必须保持一致）

- **IPC 通道（63 个）**：`rules:*`、`danmaku:*`、`conn:*`、`overlay:*`、`audio:*`、`input:*`、`serial:*`、`obs:*`、`features:*`、`auth:*`、`config:*`、`diagnostics:*`、`window:*`
- **主进程 → 渲染进程推送**：`danmaku:append`、`conn:status`、`log:append`、`overlay:message`、`overlay:status`
- **窗口内消息**：`play-video`、`drop`、`slot-start`、`component-widget`、`component-remove`、`background-mode`、`window-closed`、`stop-audio`
- **动作类型（8 种）**：`video`、`audio`、`drop`、`slot`、`serial`、`obs`、`key`、`mouse`
- **SQLite 表**：`rules`、`danmaku_records`、`logs`、`settings`

---

## 2. 目标架构（Wails v3）

> 下文 Wails API 形态取自《1比1复刻实现总设计文档-Wails版.md》；v3 仍处 Beta，具体签名以官方文档与生成代码为准。

```text
abi-replica.exe（Go + Wails v3，单进程多窗口）
├── 主窗口          普通窗口，默认 900×600，最小 760×500
├── 绿幕窗口        普通窗口 + 纯色底 #00FF00（OBS 色键）
├── 组件窗口        普通窗口 + 系统原生标题栏 + WebView2 透明客户区
└── 音频窗口        隐藏窗口（或子进程 voice.exe）
        ▲ Wails 绑定（前端 → Go）
        ▼ Wails 事件（Go → 前端）
Go 服务：Rules / Danmaku / Conn / Overlay / Audio / Input / Serial / Obs / Features / Auth / Config / Diagnostics / Window
持久化：modernc.org/sqlite（app.db，表结构照搬）
外部：自建授权服务（Node，不变）、yshow.exe（可选）、OBS WebSocket
```

目录（对齐 Wails 版设计文档 §3）：

```text
abi-replica/
├── wails.json
├── go.mod
├── main.go                 # application.New + 四个窗口
├── internal/
│   ├── windows/            # Wails 多窗口管理
│   ├── rules/              # 规则引擎（移植 rule-engine.ts）
│   ├── actions/            # 8 类动作执行
│   ├── danmaku/            # 连接器 + 记录落库
│   ├── store/              # SQLite
│   ├── obs/                # OBS WebSocket 客户端
│   ├── serial/             # 串口
│   ├── input/              # 键鼠 / 找图
│   ├── license/            # 自建授权客户端
│   └── config/             # 设置 + 导入导出
├── frontend/               # ← 现 src/renderer 整体搬入
│   └── src/
│       ├── pages/ components/ stores/ styles.css
│       └── services/api.ts # 换成 Wails 适配层（接口形状不变）
└── build/                  # 图标、NSIS 脚本
```

---

## 3. 逐模块迁移

### 3.1 窗口与透明（本次迁移的核心收益）

| 窗口 | Wails 配置 | 备注 |
|---|---|---|
| 主窗 | `Frameless: false`、`MinWidth/MinHeight: 760×500` | 默认 900×600；保留自绘标题栏 |
| 绿幕窗 | 普通窗口、`AlwaysOnTop`、不可最小化、底色 `#00FF00` | 与现状一致（色键） |
| 组件窗 | 普通窗口（**系统原生标题栏**）、WebView2 客户区透明 | 使用 Wails `BackgroundTypeTransparent` 与 alpha 0；Windows 11+ 按 Wails 当前支持范围提供透明窗口背景 |
| 音频窗 | `Hidden: true` | 或独立 `voice.exe` |

组件窗透明配置：

```go
application.WebviewWindowOptions{
    Frameless:        false, // 保留系统原生标题栏
    BackgroundType:   application.BackgroundTypeTransparent,
    BackgroundColour: application.NewRGBA(0, 0, 0, 0),
}
```

前端配合（与当前实现一致）：

- 组件窗始终使用透明 WebView2 背景，`html/body/#app/.slot-overlay` 保持透明；
- **透明底板**模式只绘制活动组件；**深色底板**模式由页面绘制 `#080d18` 面板；
- 切换入口：Ctrl+F1 全局热键 或 扩展功能页按钮；
- 不依赖黑色色键，因此组件可使用纯黑；透明像素会透出下层画面，鼠标输入仍交给组件窗，标题栏可用于拖动。

### 3.2 IPC → Wails 绑定（63 通道映射）

| 现有通道前缀 | Go 服务对象 | 方法示例 |
|---|---|---|
| `rules:*` | `RulesService` | `List/Save/Remove/Clear/Clone` |
| `danmaku:*` | `DanmakuService` | `Query/Count/Export/Clear` |
| `conn:*` | `ConnService` | `Connect/Disconnect/Status/Simulate` |
| `overlay:*` | `OverlayService` | `Open/Close/Status/SetMode/UpdateSettings/ToggleOpacity/PlayVideo/Drop/Slot/Widget/RemoveWidget` |
| `audio:*` | `AudioService` | `Play/Stop` |
| `input:*` | `InputService` | `Run/FindImage` |
| `serial:*` | `SerialService` | `Ports/Pulse/StopAll` |
| `obs:*` | `ObsService` | `Connect/Command/StartVirtualCamera/StopVirtualCamera/Status` |
| `features:*` | `FeatureService` | `Show/Test/Increment` |
| `auth:*` | `AuthService` | `Status/Login/Logout/SetSafeCode/Unbind` + 后台 `AdminStatus/AdminLogin/ListCards/CreateCards/SetCardStatus/ResetCardBindings/CardDetail/ExtendCard/AuditLog` |
| `config:*` | `ConfigService` | `Export/Import` |
| `diagnostics:*` | `DiagnosticsService` | `Logs/Assets/Settings/SaveSettings` |
| `window:*` | `WindowService` | `Minimize/Maximize/Close` |

注册：

```go
app := application.New(application.Options{
    Services: []application.Service{
        application.NewService(&RulesService{store: ruleStore}),
        application.NewService(&DanmakuService{store: danmakuStore}),
        application.NewService(&ConnService{mgr: connMgr}),
        application.NewService(&OverlayService{wm: winMgr}),
        application.NewService(&ObsService{client: obsClient}),
        // …其余同构
    },
})
```

### 3.3 推送 → Wails 事件（事件名保持不变）

| 现有通道 | 方向 | Wails 对应 |
|---|---|---|
| `danmaku:append` | Go → 主窗 | `app.EventsEmit(ctx, "danmaku:append", record)` |
| `conn:status` | Go → 主窗 | 同上 |
| `log:append` | Go → 主窗 | 同上 |
| `overlay:status` | Go → 主窗 | 同上 |
| `overlay:message` | Go → 绿幕/组件/音频窗 | 对应窗口 `EmitEvent("overlay:message", msg)`（同进程直接调用，不再走 IPC） |

### 3.4 存储：sql.js → `modernc.org/sqlite`

- 表结构照搬（现有 DDL，直接用）：

```sql
CREATE TABLE IF NOT EXISTS rules (id TEXT PRIMARY KEY, json TEXT NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS danmaku_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL, room_id TEXT, kind TEXT NOT NULL,
  user_id TEXT, user_name TEXT, text TEXT, gift_name TEXT, /* …礼物数/连击/命中规则/执行结果/ts/created_at */
);
CREATE INDEX IF NOT EXISTS idx_danmaku_ts ON danmaku_records(ts);
CREATE INDEX IF NOT EXISTS idx_danmaku_source_ts ON danmaku_records(source, ts);
CREATE INDEX IF NOT EXISTS idx_danmaku_kind_ts ON danmaku_records(kind, ts);
CREATE INDEX IF NOT EXISTS idx_danmaku_user ON danmaku_records(user_name);
CREATE TABLE IF NOT EXISTS logs (id INTEGER PRIMARY KEY, level TEXT NOT NULL, category TEXT NOT NULL, message TEXT NOT NULL, detail TEXT);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
```

- 数据目录：`%APPDATA%/阿比整蛊复刻版/data/app.db`，同级还有 `logs/`、`configs/`；现有 `app.db` 是标准 SQLite 文件，**可直接沿用**
- 设置项 `AppSettings` 仍以 JSON 存在 `settings` 表，`overlaySlot.backgroundTransparent` 等字段沿用

### 3.5 规则引擎：`rule-engine.ts` → `internal/rules`

移植语义（照抄现实现，别重设计）：

- `matchRule(rule, event)`：事件类型 ∈ `trigger.kinds`；礼物名/关键词（`contains`/`exact`/`regex`）；`minCount`；`users` 白名单；`source` 过滤
- 调度：按 `priority` 降序匹配、`cooldownMs` 冷却、`probability` 概率、`concurrency` ∈ `queue|parallel|replace|exclusive`
- 事件队列：并发消费、丢弃过期事件、`dropped` 计数（现有 `ConnectorStatus.dropped`）
- 自检用例（`scripts/self-check.ts`）1:1 转成 Go 表驱动测试

### 3.6 动作执行：`action-executor.ts` → `internal/actions`

| 动作 | Go 实现要点 |
|---|---|
| `video` | 绿幕窗 `EmitEvent("play-video")`，前端 `<video>` 播放（多路 = `videos/<n>/` 目录） |
| `audio` | 同进程播放器或 `voice.exe` 子进程；音量/打断/循环 |
| `drop` | 绿幕窗 `EmitEvent("drop")`，前端 CSS/Canvas 下落动画（重力/弹跳/数量） |
| `slot` | 组件窗 `EmitEvent("slot-start")`，前端转盘 + 加权结果 |
| `serial` | `go.bug.st/serial`：开/关字节、脉冲时长 |
| `obs` | OBS WebSocket 请求（滤镜、静音、场景） |
| `key` / `mouse` | `user32.SendInput`：组合键、按住时长、序列、绝对/相对坐标；目标窗口按标题/类名绑定 |

### 3.7 前端适配层（关键：前端零改动）

`src/renderer/services/api.ts` 保持 `ElectronApi` 形状，只把实现换成 Wails：

```ts
// frontend/src/services/api.ts（迁移后）
import * as Rules from '../wailsjs/go/main/RulesService'
import * as Overlay from '../wailsjs/go/main/OverlayService'
import { EventsOn } from '../wailsjs/runtime'

export const api: ElectronApi = {
  rules: {
    list: () => Rules.List(),
    save: (rule) => Rules.Save(rule),
    remove: (id) => Rules.Remove(id),
    clear: () => Rules.Clear(),
    clone: (id) => Rules.Clone(id),
  },
  danmaku: {
    query: (f) => Danmaku.Query(f),
    // …
    onAppend: (cb) => { EventsOn('danmaku:append', cb); return () => { /* EventsOff */ } },
  },
  overlay: {
    playVideo: (p) => Overlay.PlayVideo(p),
    // …
  },
  // 其余命名空间同构
}
```

其余页面、组件、样式、`stores/app.ts`、`router.ts` **全部原样搬运**。

### 3.8 授权：独立 Node 后端 + Wails 用户客户端

- 原授权服务迁至 `backend/` 独立项目部署，保留 `/v1/auth/*`、`/v1/admin/*`、`/health` 协议，并提供签名更新的 `/v1/updates/*` 接口
- Go 侧实现用户授权 HTTP 客户端；**桌面端不打包管理员密码、卡密 pepper 或管理员 API 页面**，会话令牌只放内存
- 桌面端只负责卡密登录与用户自身的安全码/设备解绑，验证窗只输入卡密；发行构建通过 `LIVETOOL_LICENSE_SERVER_URL` 固定授权服务 HTTPS 地址。真实域名待部署配置，正式发行构建需在设置该变量后执行
- 发卡、状态管理、设备重置、审计由独立服务管理员 API 负责
- 现有「本地开发模式」（仅非 production 构建）保留；正式版必须验证远端卡密

### 3.9 日志、诊断、配置导入导出

- `logger`：分级 + 落 `logs` 表 + 事件推送（`log:append`）
- `diagnostics:assets`：素材根目录扫描（`resources/`）
- `config:export/import`：规则 + 设置 + 功能参数的 JSON 包（结构与现在一致，便于两边互导）

### 3.10 打包

- `wails build -platform windows/amd64` → 单 exe（无 Chromium 分发，体积远小于 Electron）
- 资源目录沿用：`resources/`（素材）、`水果机/`、`videos/`、`voices/`、`images/`
- 安装器：`wails3 task windows:package` 生成 Windows amd64 exe 与 NSIS 用户级安装包；安装器按需设置 WebView2 Evergreen Runtime，卸载保留 `%APPDATA%/阿比整蛊复刻版` 用户数据
- 绿幕和组件 Overlay 窗标题保留「禁止最小化」提示，且禁用系统最小化/最大化按钮，方便 OBS 按标题选择窗口

---

## 4. 执行顺序（每步都能跑起来）

| 阶段 | 内容 | 验收 |
|---|---|---|
| A. 骨架 | `wails init` + 前端搬入 + 主窗跑通 | 主界面与现在一致，能开 DevTools |
| B. 数据 | SQLite + 设置 + 规则 CRUD + 规则引擎 + 自检 | `go test ./internal/rules` 全绿；规则可增删改查 |
| C. 窗口 | 绿幕/组件/音频窗 + 消息协议 + **组件窗透明背景** | Windows 目标机：标题栏仍在、空白客户区显示下层内容、活动组件不透明；OBS 窗口采集打开透明度后只剩组件 |
| D. 动作 | 键鼠、音频、砸落物、水果机、串口、OBS | 记事本验证按键；OBS 验证滤镜/虚拟摄像头；串口回环 |
| E. 授权与发布 | 自建授权客户端 + 卡密后台 + 打包 | 卡密登录/绑定/解绑；安装包可在干净机器运行 |

---

## 5. 复用 / 重写 / 删除清单

| 处理 | 对象 |
|---|---|
| **直接复用** | `src/renderer/**`（页面、组件、样式、store、router）、`src/shared/types.ts`、`src/shared/features.ts`、原授权服务协议（实现迁至 `backend/` 并增加签名更新接口）、`resources/**`、配置 JSON 结构 |
| **重写** | `src/main/**`（→ Go）、`src/preload/**`（删除，改绑定）、`electron.vite.config.ts`（→ `wails.json`）、打包配置 |
| **删除** | electron / electron-builder / electron-vite / sql.js / koffi（若之前引入）相关依赖 |
| **保留思路** | `scripts/self-check.ts` 的用例 → Go 单测 |

---

## 6. 风险与验证方法

| 风险 | 验证 / 缓解 |
|---|---|
| Wails v3 处于 Beta，绑定路径与 `WebviewWindowOptions` 可能微调 | 锁定版本；绑定层集中在 `services/api.ts` 一处，便于跟进改动 |
| Wails/WebView2 的透明背景受 Windows 版本与窗口样式影响；透明区域输入仍由组件窗接收 | 锁定 Wails 版本；在目标 Windows 版本检查原生标题栏、客户区透明、组件绘制和 OBS alpha；保持 `IgnoreMouseEvents: false` |
| WebView2 与 BASS 音频冲突 | 先同进程（`oto`/`beep`），有冲突再拆 `voice.exe` 子进程 |
| 键鼠需要管理员权限（目标程序提权时） | `input` 拆独立子进程，按需以管理员启动 |
| 多窗口 DevTools / 高 DPI | `wails dev` 逐窗口验证；DPI 用 PerMonitorV2 |
| 现有数据迁移 | `app.db` 直接读；必要时写一次性迁移命令 |

验证组件窗透明是否生效（Windows 侧）：

```powershell
# 1) 窗口样式
#    期望：WS_CAPTION=True（原生标题栏）、Frameless=False；不设置 WS_EX_LAYERED 黑色色键
# 2) 屏幕观察：窗口空白客户区显示下层窗口/桌面内容，组件区域仍是组件自身颜色
# 3) 切换底板：Ctrl+F1 在透明背景和页面绘制的深色面板之间切换
# 4) OBS：窗口采集勾选「允许透明度」后应只看到活动组件
```

---

## 7. 验收清单（对齐 Wails 版设计文档 §20）

以下项目保持未勾选，直到原生 Windows amd64 目标机完成可视界面、窗口、OBS、更新安装和设备行为验收。Windows ARM64 虚拟机的兼容层冒烟不替代该验收；构建或静态检查也不能代替运行验收。细分验证证据见 `docs/implementation-evidence.md`。

- [ ] 主窗、绿幕窗、组件窗、音频窗行为与 Electron 版一致
- [ ] 组件窗：**系统原生标题栏** + WebView2 透明客户区 + 活动组件不透明（Ctrl+F1 切底板）
- [ ] 绿幕窗：纯绿底、色键后只剩特效；多路视频可同时播
- [ ] 事件闭环：模拟事件 → 规则匹配 → 视频/音频/砸图/水果机/键鼠动作
- [ ] 弹幕日记：实时 + 历史检索 + 导出 + 清理
- [ ] 18 个扩展功能开关与参数持久化
- [ ] 卡密登录 / 安全码 / 解绑 / 后台发卡（自建服务）
- [ ] 配置导入导出可跨版本互认
- [ ] 打包产物在干净 Windows 上可运行，OBS 可稳定采集

---

## 附：迁移后「不变」的契约（写代码时的红线）

1. **事件与通道名不变**：`danmaku:append`、`conn:status`、`log:append`、`overlay:message`、`overlay:status`、`play-video`、`drop`、`slot-start`、`component-widget`、`component-remove`、`background-mode`。
2. **数据结构不变**：`LiveEvent` / `Rule` / `Action` / `DanmakuRecord` / `AppSettings` / `OverlaySettings` 字段与 JSON 命名保持一致（前后端与配置文件互通）。
3. **素材目录契约不变**：`resources/`、`videos/<n>/`、`voices/`、`images/`、`水果机/*.png`。
4. **授权协议不变**：只换客户端语言，端点、字段、HMAC 规则与自建服务保持一致。
