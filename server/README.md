# 自建卡密授权服务

服务端与 Electron 客户端分开运行。客户端不会保存管理员密码、卡密摘要密钥或管理员令牌；卡密明文只在批量生成时返回一次。

## 启动

需要 Node.js 20 或更新版本。在服务机上设置以下环境变量后，从项目根目录运行 `npm run auth-server`：

| 环境变量 | 必填 | 说明 |
| --- | --- | --- |
| `CARD_ADMIN_USERNAME` | 是 | 后台管理员账号 |
| `CARD_ADMIN_PASSWORD` | 是 | 后台管理员密码，至少 12 个字符 |
| `CARD_KEY_PEPPER` | 是 | 至少 32 字节的随机秘密；必须长期保密且备份，丢失后现有卡密无法验证 |
| `CARD_HOST` | 否 | 默认 `127.0.0.1`；远程部署优先由同机 HTTPS 反向代理转发 |
| `CARD_PORT` | 否 | 默认 `8787` |
| `CARD_DATA_FILE` | 否 | 卡密数据 JSON 文件；默认项目根目录下的 `license-data/cards.json` |

Linux/macOS 示例（请在受控的服务终端中执行）：

```sh
export CARD_ADMIN_USERNAME='admin'
export CARD_ADMIN_PASSWORD='替换为独立的强密码'
export CARD_KEY_PEPPER="$(openssl rand -hex 32)"
export CARD_DATA_FILE='/var/lib/abizhenggu/cards.json'
npm run auth-server
```

Windows PowerShell 示例：

```powershell
$env:CARD_ADMIN_USERNAME = 'admin'
$env:CARD_ADMIN_PASSWORD = Read-Host '设置管理员密码（至少 12 个字符）'
$env:CARD_KEY_PEPPER = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
$env:CARD_DATA_FILE = 'D:\AbizhengguData\cards.json'
npm run auth-server
```

远程使用时，将 `https://你的域名` 配置在客户端“卡密管理”或“授权 / 模式”页，并由 Nginx、Caddy 等反向代理提供 TLS。服务本身只提供 HTTP；不要把明文 HTTP 端口直接暴露到公网，也不要将 `CARD_KEY_PEPPER` 写入仓库或客户端配置。

## 后台功能

- 管理员登录（服务端内存会话，8 小时过期，按来源地址限制连续失败登录）。
- 批量生成 1–1000 张卡密；每张卡可配置首次激活后的有效天数、设备数量、平台、功能权限和备注。
- 查询/筛选卡密、查看设备绑定摘要、续期、停用、重新启用、永久撤销、重置全部设备绑定及查看审计记录。
- 服务端仅保存带 `CARD_KEY_PEPPER` 的 HMAC 摘要，不保存可回显的卡密明文。
- 用户授权会话为短期滑动会话；授权状态接口会重新检查卡密状态、有效期和设备绑定。

设备绑定使用客户端首次启动时生成并保存在本机应用数据库中的随机安装标识，不是硬件指纹。清除/迁移应用数据可能改变该标识；管理员可在后台重置绑定。

## API 概览

- `POST /v1/admin/login`、`GET /v1/admin/me`
- `GET/POST /v1/admin/cards`、`GET /v1/admin/cards/:id`
- `PATCH /v1/admin/cards/:id/status`、`DELETE /v1/admin/cards/:id/bindings`、`POST /v1/admin/cards/:id/extend`
- `GET /v1/admin/audit`
- `POST /v1/auth/login`、`GET /v1/auth/status`、`POST /v1/auth/logout`
- `POST /v1/auth/safe-code`、`POST /v1/auth/unbind`
- `GET /health`

## 运维边界

- 当前存储为单进程 JSON 文件；同一数据文件只能由一个服务进程读写。部署多实例前需迁移到具备事务和共享会话的数据库/存储。
- `CARD_DATA_FILE` 与 `CARD_KEY_PEPPER` 都应纳入服务端备份；限制数据文件与环境变量的操作系统访问权限。
- 管理员和用户会话保存在内存中，服务重启后需重新登录；卡密与审计数据保存在数据文件中。
- 卡密撤销不可恢复；明文遗失后需撤销并重新生成。
