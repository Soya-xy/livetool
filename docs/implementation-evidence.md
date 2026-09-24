# 实现与验证边界

## 本次迁移

桌面运行入口现为 Wails v3（Go + Vue 3）。Go 服务持有 SQLite、规则调度、模拟器连接、18 个功能、OBS WebSocket、配置导入导出、日志、诊断、设备卡密会话和 Wails 更新器。卡密与更新服务独立部署在 `backend/`；管理员 API 只在后端，不进入桌面前端。SQLite 与日志配置目录沿用 Electron `productName` `阿比整蛊复刻版`，继续读取同一 `%APPDATA%/阿比整蛊复刻版/data/app.db`。

开发构建保留原版的本地开发授权，生产构建通过 `production` build tag 关闭本地模式；弹幕记录补齐了礼物名称列映射，日志使用 SQLite 自增 ID，避免同一毫秒的日志相互覆盖。

桌面主窗默认改为 900×600（最小 760×500），界面固定为浅色：白色卡片、浅冷灰工作区、低饱和靛蓝主色与状态色；没有暗黑模式。扩展功能使用紧凑卡片，控制中心合并主要操作与搜索栏、表格按内容伸缩且最多 300px。卡密、模拟器、功能设置和规则编辑器弹窗宽度分别为 380、460、560 和 700px，并保留内容滚动与固定底部操作。规则编辑器为动作参数补字段标签、结构化键鼠步骤、串口选择/刷新与真实 I/O 提示。键盘焦点和跳到主内容链接保持可见；紧凑布局中的表单和说明文字至少 11–12px，并支持减少动态效果。叠加窗保留原生标题栏并禁用最小化/最大化；视频、音频和图片改走同源本地素材路由，支持媒体 Range 请求并检查符号链接不越过素材根目录。

卡密弹窗只要求输入卡密，桌面端不再让用户填写授权服务器地址或直播平台。授权服务地址由生产构建变量 `LIVETOOL_LICENSE_SERVER_URL` 固定注入；生产构建忽略旧配置中的服务器地址，开发构建仍可用本地 `127.0.0.1:8787`。当前还没有收到实际 HTTPS 域名，因此可编译和完成本地服务联调，但真实生产卡密验证与正式发布构建仍待部署域名。

直播平台连接仍使用本地模拟器；Electron 原版的各平台适配器未实现。键鼠动作现在在 Windows amd64 上调用 `user32.SendInput`，支持按键/鼠标步骤与按窗口标题、类名或进程名聚焦；串口动作通过 `go.bug.st/serial` 枚举端口并写入配置的开启/关闭字节。两条真实 I/O 路径已交叉编译，但尚未在 Windows 桌面、目标窗口或物理串口上运行验收。本次没有实际发送键鼠输入或触发串口设备。查图接口沿用 Electron 原版的“未执行”响应。

此前组件窗用 Win32 黑色色键处理窗口背景，但该方式不能保证对 WebView2 客户区生效，现已移除。组件窗改用 Wails `BackgroundTypeTransparent` 与 alpha 0 的 WebView2 背景，页面负责绘制深色底板模式；保持普通窗口样式以保留系统标题栏。绿幕、组件窗标题恢复「禁止最小化」提示，系统最小化/最大化按钮禁用。Windows 目标机可视透明效果和 OBS alpha 采集仍待验收。

## 当前验证

- `go run ... wails3 generate bindings -ts -i -d frontend/bindings`：通过，生成 1 个服务、59 个方法、3 个枚举和 30 个模型。
- `npm --prefix frontend run check`：通过。
- `npm --prefix frontend run build`：当前主 JS chunk 335.43 kB（gzip 120.09 kB），无 500 kB chunk 警告；Element Plus 组件和路由按需加载。
- macOS 生产 UI 冒烟（900×600）：浅色改版后检查控制中心、弹幕日记、通用设置、扩展设置、规则编辑器和卡密窗口；隔离服务签发的临时卡密通过桌面 UI 验证并显示 `overlay`、`slot`、`serial` 权限。无授权时模拟事件被拦截；授权后发送的模拟礼物出现在弹幕日记。测试窗口、服务及隔离数据已关闭并清理。此项覆盖 macOS UI 与本地服务联调，不代替 Windows 原生验收。
- `go build ./...`：通过；macOS linker 提示部分依赖以高于本机默认 deployment target 构建。
- `wails3 task build`：完整生产构建通过，包含绑定生成、前端打包和 macOS 程序链接；链接器提示部分依赖的 macOS deployment target 较高。
- 早前的 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags production ...`：通过，生成 19 MB PE32+ GUI 产物；当时的版本仍包含现已删除的黑色色键实现，因此不作为当前透明实现的构建证据。
- Windows 桌面可视冒烟：将该 x64 产物在 Parallels 的 Windows 11 ARM64 虚拟机中通过兼容层启动，主窗口进程保持响应；实际查看了浅色控制中心和扩展页，并确认键盘跳转焦点、卡密验证弹窗开关。未验证时，设置入口按预期提示先验证卡密。运行前将 `APPDATA`、`LOCALAPPDATA` 和临时目录指向专用 QA 路径，确认 `app.db` 与 WebView2 数据位于该路径；未输入真实卡密，也未改动常规用户配置。该结果证明兼容层下的启动与基础界面，不等同于原生 Windows amd64 目标机验收。
- `go test ./...`：通过编译，Go 包当前没有保留测试文件；临时检查覆盖串口参数、Electron 原表结构数据库读取、媒体 URL 编码/Range 响应/目录穿越与符号链接边界，测试文件及临时数据库均已清理。
- `go vet ./...`：通过。
- 临时配置导入冒烟检查（运行后删除）：旧版本只包含部分 overlay 字段时保留当前未提供的透明度、置顶与背景设置；显式 `store_raw: false` 和空 `features` 对象都按字段存在语义处理。
- 设置合并冒烟检查（临时检查文件运行后删除）：全局音量设为 `0` 后保存、重载仍保持静音；旧配置缺少 `audioVolume` 时仍使用 `0.8` 默认值。修复了原先把静音误判为缺省值的问题。
- `git diff --check`：通过；仓库运行文件中未发现 Electron 运行时依赖或引用。
- 临时 Go 冒烟检查（运行后删除）：开发/production 两种 build tag 的本地授权门禁；模拟器连接 → 礼物事件 → 规则匹配（数量、用户、礼物名、正则）→ 冷却跳过 → SQLite 日记结果通过。那次键鼠日志只验证了旧的模拟处理器，不构成当前 Windows `SendInput` 的执行证据。
- 当前功能与资源路径冒烟（临时检查文件和 SQLite 数据库均已清理）：礼物咖路径越界时功能测试返回错误；`OverlaySlot` 接收 `[]string` 与 `[]any` 两种图片数组时均拒绝越界路径，缺失音频也返回明确错误。静态检查确认组件预览会返回 Overlay 错误，弹幕触发失败会记录警告而不写成功日志。
- `node --check backend/index.mjs`：通过。
- 卡密服务本地 API 冒烟：健康检查、管理员登录、发卡/列表、卡密验证和设备绑定、授权状态、安全码解绑与会话撤销、更新鉴权/清单/产物下载及明文卡密不落盘均通过；临时数据已清理。
- Wails 更新签名闭环：使用临时产物生成签名清单，并由嵌入公钥验证摘要和 Ed25519ph 签名通过。
- Wails 更新 provider 联调（临时卡密服务、卡密、清单、产物和 Go 冒烟检查文件均在结束后清理）：将当前生产 Windows amd64 PE 产物签为 `0.2.1`；`licenseUpdateProvider.Check` 携带卡密会话选中匹配清单，`Download` 从授权服务取回完整 19.8 MB PE 文件；SHA-512 与清单一致，Ed25519ph 签名由当前嵌入公钥验过。此项覆盖 Windows 产物、provider、服务鉴权、下载和签名校验，不覆盖 Windows 原生替换、重启或目标机运行。
- 本轮复验：`wails3 task build`、`go vet ./...`、Windows amd64 production 交叉编译、Vue 类型检查、后端 Node 语法检查和 `git diff --check` 均通过。生产设置合并强制关闭 `devMode`，调试快捷键入口也有 build tag 检查；键盘跳转主内容的可见焦点样式不再被旧规则覆盖，礼物配置字段、窗口开关和功能卡片开关均有明确辅助标签。Vite 曾提示约 1.19 MB 主 JS chunk，macOS 链接仍提示依赖的 deployment target 高于本机构建目标。
- 前端按需加载：移除全局 Element Plus 组件注册和全量样式，使用组件自动导入与页面异步路由；`ElMessage` / `ElMessageBox` 的服务样式仍显式载入。`npm run check` 和完整 Wails 生产构建通过，主 JS chunk 从先前约 1.19 MB 降至 335.43 kB（gzip 120.09 kB），不再触发 500 kB chunk 警告；页面脚本和组件样式按需拆分。固定浅色配色未变。本次资源加载修改没有重新做原生 Windows GUI 验收。
- 连接器防误报：检查发现 Go 服务会把任意已知平台都标成已连接的模拟器；现改为只接受 `simulator`，其他尚未实现的平台返回明确错误，并规范化直播间 ID。`go vet ./...`、`go build ./...` 通过；macOS 链接仍有依赖最低系统版本高于本机构建目标的警告。原生平台连接能力不属于这项修复。
- 紧凑浅色 UI 复核：将主界面说明、表格和表单文本最低字号统一到 11–12px，辅助文字改用更高对比度的灰蓝色；保留 900×600 默认窗口和 760×500 最小尺寸。实时弹幕行增加 `content-visibility` 以降低 500 条缓存列表的屏外绘制成本。`npm run check`、完整 Wails 构建、Windows amd64 production 交叉编译、`go vet ./...` 和 `git diff --check` 通过；未重新做 GUI 可视验收。
- 卡密入口精简：授权服务器地址与平台选择从用户弹窗移除；开发版本仍默认连本地授权服务，生产版本只接受发行构建注入的 HTTPS 地址，并拒绝 Electron 旧配置覆盖。Taskfile 的生产打包现要求提供 `LIVETOOL_LICENSE_SERVER_URL`。实际部署域名尚未提供，本轮不能宣称生产卡密验证已完成。
- 2026-09-24 复验：Vue 类型检查、Vite 生产构建、`go vet ./...`、macOS `go build ./...`、Windows amd64 production 交叉编译、后端 Node 语法检查和 `git diff --check` 均通过。完整 Wails Taskfile 生产构建也通过；为满足新构建门禁使用 `https://license.test.invalid` 仅做编译验证，并逐字节恢复原有 `bin/livetool` 文件。macOS linker 仍提示部分依赖以 macOS 13/27 为目标，而当前链接目标为 11.0。
- 2026-09-24 桌面视觉复验未完成：Computer Use 返回 macOS 已锁定，无法打开 Wails 主窗；本轮没有针对最新卡密弹窗和字号 CSS 截图。`lh` acceptance CLI 未安装，因此不能发布带 UI 证据的验收轮次。此前记录的 macOS 和 Windows ARM64 兼容层冒烟仅代表当时版本，不替代这轮视觉复核。
- 2026-09-24 独立服务运行时复验：在临时目录和随机本地端口启动 `backend/index.mjs`，验证健康检查、管理员登录、发卡与卡密明文不落盘、设备授权/权限返回、安全码设置、解绑及会话撤销、更新清单鉴权和更新文件下载；进程退出并删除临时卡密数据与更新文件。
- 2026-09-24 功能权限契约复核：对比 Electron `src/shared/license.ts`、Wails Go 授权门禁、18 个前端功能 ID、Go 默认配置和执行分支，发现并修正 `impact-gift` / `gift-screen` 与 `danmaku-assistant` / `mosquito-slap` 的 entitlement 映射漂移。静态契约检查通过；`go vet ./...`、`go build ./...`、Windows amd64 production 交叉编译、Vue 类型检查及完整 Wails Taskfile 构建通过。完整构建仍仅以 `https://license.test.invalid` 验证链接，不代表真实授权端点。
- 2026-09-24 组件窗透明实现：移除 `FindWindowW` / `SetLayeredWindowAttributes` 色键调用，组件窗固定使用透明 WebView2 客户区与 alpha 0，系统标题栏仍由普通 Wails 窗口保留；透明/深色底板切换继续走现有 Ctrl+F1 与扩展页按钮，深色背景由前端绘制。`go vet ./...`、`go build ./...`、Windows amd64 production 交叉编译、Vue 类型检查、Vite 构建和 `git diff --check` 均通过。当前环境没有原生 Windows 桌面，不能把源码配置与交叉编译视为透明效果或 OBS 采集验收。
- 2026-09-24 事件队列补齐：连接事件进入最多 1000 条的 FIFO 队列，由 4 个 worker 并发消费；同一用户/事件内容在 3 秒内去重，队列满时礼物可替换最早的非礼物，积压 30 秒的事件会丢弃并增加 `dropped`。退出时取消等待中的动作延迟并等活动 worker 结束后再关闭 SQLite。临时 SQLite 冒烟覆盖过期事件丢弃、重复事件只落一条记录、队列溢出时的礼物优先和 `dropped` 计数；临时检查文件已删除。
- 2026-09-24 UI 可访问性复核：规则编辑器动作摘要改为原生按钮，可键盘聚焦切换对应参数区，并用 `aria-controls` / `aria-expanded` 说明展开关系；按下态、可见焦点和 reduced-motion 样式保留。最终主题主色由 `--el-color-primary: #5969d9` 使用低饱和靛蓝，`color-scheme: light` 固定浅色；没有暗黑主题。Vue 类型检查与 Vite 构建通过。当前主机锁定，未对这版做窗口截图目视验收。
- 2026-09-24 事件队列并发冒烟：临时 SQLite 检查并发提交 64 个不同事件，所有事件均落库；`go test -race` 未报告数据竞争。临时检查文件已删除。
- 2026-09-24 最终源码复验：`go test ./...`（当前无保留测试文件）、`go vet ./...`、macOS `go build ./...`、Windows amd64 production 交叉编译、Vue 类型检查、Vite 生产构建、后端 Node 语法检查和 `git diff --check` 均通过。macOS 链接仍提示部分依赖面向 macOS 13/27，而当前链接目标为 11.0。
- 2026-09-24 Windows 安装包链路：加入 `windows:build` / `windows:package` Taskfile 任务和 NSIS 脚本；按需调用 Wails v3.0.0-beta.25 的 WebView2 bootstrapper 生成命令，使用用户级安装与卸载登记，保留 `%APPDATA%/阿比整蛊复刻版` 数据。安装 NSIS 3.12 后，以 `https://license.test.invalid` 执行了完整打包，得到 18 MB PE32+ GUI 程序和 10.8 MB NSIS 安装器；测试产物与 bootstrapper 已清理。真实卡密服务域名尚未提供，目标 Windows 上实际安装、启动、WebView2 初始化仍待验收；Taskfile 会拒绝缺少 NSIS 或非 HTTPS 授权地址的发行构建。

未在仓库保留测试文件。macOS 主界面与 Windows 11 ARM64 虚拟机中的 x64 兼容层界面均做了可视冒烟；当前仍没有原生 Windows amd64 机器验收。键鼠/串口的代码与交叉编译检查不能证明 Windows 权限、窗口焦点、坐标或真实设备行为正确。Windows 冒烟使用隔离 `APPDATA`，尚未在原生 Windows 用户目录验证既有 `app.db` 迁移。构建和 API 检查不能替代 OBS 采集、真实串口/键鼠、更新安装或多设备授权验收。
