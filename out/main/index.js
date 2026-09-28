import { screen, BrowserWindow, app, dialog, globalShortcut, ipcMain, session } from "electron";
import { existsSync } from "node:fs";
import { mkdir, appendFile, readFile, writeFile, rename, readdir } from "node:fs/promises";
import { join, dirname, resolve, basename, isAbsolute, sep, extname, relative } from "node:path";
import { randomUUID, createHash } from "node:crypto";
import { createRequire } from "node:module";
import { pathToFileURL, fileURLToPath } from "node:url";
import initSqlJs from "sql.js";
import __cjs_mod__ from "node:module";
const __filename = import.meta.filename;
const __dirname = import.meta.dirname;
const require2 = __cjs_mod__.createRequire(import.meta.url);
const text = (key, label, defaultValue, placeholder, help) => ({ key, label, type: "text", defaultValue, placeholder, help });
const number = (key, label, defaultValue, min, max, step, help) => ({ key, label, type: "number", defaultValue, min, max, step, help });
const color = (key, label, defaultValue) => ({ key, label, type: "color", defaultValue });
const toggle = (key, label, defaultValue, help) => ({ key, label, type: "boolean", defaultValue, help });
const select = (key, label, defaultValue, options) => ({ key, label, type: "select", defaultValue, options });
const FEATURE_DEFINITIONS = [
  {
    id: "green-window",
    name: "绿幕窗口",
    description: "一个绿色的窗口，用于展示视频、组件等效果",
    icon: "VideoCamera",
    accent: "#0b5d1e",
    fields: [text("displayContent", "显示内容", "视频组件", "例如：视频组件"), text("videoPath", "测试视频", "", "选择素材目录中的视频路径"), number("videoDurationMs", "播放时长(ms)", 8e3, 100, 6e5), toggle("videoLoop", "循环播放", false), color("backgroundColor", "绿幕颜色", "#00ff00"), number("laneCount", "显示通道", 3, 1, 8), toggle("alwaysOnTop", "窗口置顶", true)]
  },
  {
    id: "component-window",
    name: "组件窗口",
    description: "用于显示组件的窗口，如：电子木鱼等",
    icon: "Grid",
    accent: "#ff4650",
    badge: "组件",
    fields: [text("displayContent", "显示内容", "组件内容", "例如：电子木鱼"), number("width", "窗口宽度", 960, 240, 3840), number("height", "窗口高度", 540, 160, 2160), toggle("alwaysOnTop", "窗口置顶", true)]
  },
  {
    id: "virtual-camera",
    name: "虚拟摄像头",
    description: "用于显示组件的摄像头输出",
    icon: "Camera",
    accent: "#22c6c9",
    badge: "new",
    fields: [text("deviceName", "设备名称", "阿比虚拟摄像头"), select("resolution", "分辨率", "1920x1080", [{ label: "1920 × 1080", value: "1920x1080" }, { label: "1280 × 720", value: "1280x720" }]), number("fps", "帧率", 30, 1, 60)]
  },
  {
    id: "speed-curve",
    name: "加速度(曲线)",
    description: "通过设置队列到达数量，让事件加速完成",
    icon: "Odometer",
    accent: "#1598dc",
    fields: [number("targetCount", "目标数量", 10, 1, 9999), number("durationMs", "完成时长(ms)", 3e3, 100, 6e5), select("curve", "曲线", "ease-out", [{ label: "缓出", value: "ease-out" }, { label: "线性", value: "linear" }, { label: "缓入缓出", value: "ease-in-out" }])],
    giftRules: true
  },
  {
    id: "speed-iba",
    name: "加速度(IB)",
    description: "通过设置队列到达数量，让事件加速完成",
    icon: "Odometer",
    accent: "#4d8da8",
    badge: "组件",
    fields: [number("targetCount", "目标数量", 10, 1, 9999), number("durationMs", "完成时长(ms)", 3e3, 100, 6e5), number("batchSize", "批量数量", 1, 1, 100)],
    giftRules: true
  },
  {
    id: "impact-gift",
    name: "砸礼物",
    description: "收到礼物将礼物砸到屏幕内",
    icon: "Present",
    accent: "#ff8068",
    badge: "组件",
    giftRules: true,
    fields: [text("imagePath", "礼物素材", "images/平底锅.png"), number("count", "数量", 8, 1, 100), number("gravity", "重力", 1800, 0, 5e3), number("bounce", "反弹", 0.45, 0, 1, 0.05)]
  },
  {
    id: "frying-pan",
    name: "煮播血条",
    description: "设置煮播血条，增加互动性",
    icon: "Dish",
    accent: "#f6d765",
    fields: [text("displayContent", "显示内容", "煮播血条"), number("maxValue", "血量上限", 100, 1, 99999), { ...number("damagePerGift", "血量变化", 10, 1, 9999), giftOnly: true }, color("barColor", "血条颜色", "#ff4f56")],
    giftRules: true
  },
  {
    id: "voice-broadcast",
    name: "语音播报",
    description: "朗读直播间弹幕、礼物等信息",
    icon: "Microphone",
    accent: "#f35b9d",
    muted: true,
    fields: [text("audioPath", "测试音频", "voices/测试音效.mp3"), text("template", "礼物播报模板", "{user} 送出 {gift}，数量 {count}", "支持 {user}、{text}、{gift}、{count}"), text("chatTemplate", "弹幕播报模板", "{user} 说 {text}", "支持 {user}、{text}、{gift}、{count}"), toggle("chatEnabled", "播报弹幕", true), number("volume", "音量", 0.8, 0, 1, 0.05), toggle("interrupt", "打断当前声音", false)],
    giftRules: true
  },
  {
    id: "danmaku-assistant",
    name: "弹幕助手",
    description: "查看直播间弹幕、礼物等信息",
    icon: "ChatDotRound",
    accent: "#4edab6",
    badge: "组件",
    muted: true,
    fields: [text("keywords", "关键词", "666, 欧皇"), select("keywordMode", "匹配方式", "contains", [{ label: "包含关键词", value: "contains" }, { label: "完全一致", value: "exact" }]), text("replyTemplate", "回复模板", "收到 {user}"), number("cooldownMs", "冷却(ms)", 3e3, 0, 6e5)]
  },
  {
    id: "live-clock",
    name: "加班时钟",
    description: "直播间下播倒计时，加班时钟",
    icon: "AlarmClock",
    accent: "#30c9d3",
    badge: "组件",
    fields: [text("displayContent", "标题", "直播倒计时"), number("durationSeconds", "倒计时(秒)", 60, 1, 86400), { ...number("secondsPerGift", "时间增量(秒)", 10, 0, 86400), giftOnly: true }, select("style", "样式", "digital", [{ label: "数字", value: "digital" }, { label: "圆环", value: "ring" }])],
    giftRules: true
  },
  {
    id: "electronic-woodfish",
    name: "电子木鱼",
    description: "电子木鱼，大家一起来集功德",
    icon: "Trophy",
    accent: "#f5a64b",
    badge: "组件",
    fields: [text("audioPath", "敲击音效", "fruit.mp3"), number("meritPerClick", "点击功德", 1, 1, 999), { ...number("meritPerGift", "功德增量", 1, 1, 999), giftOnly: true }, toggle("showCounter", "显示计数", true)],
    giftRules: true
  },
  {
    id: "slot-machine",
    name: "水果机",
    description: "休闲水果机，水果机盲盒",
    icon: "PictureRounded",
    accent: "#da4da4",
    fields: [text("theme", "主题", "default"), text("pool", "奖项池", "一等奖, 二等奖, 谢谢参与"), text("weights", "权重", "1, 10, 89"), number("durationMs", "动画时长(ms)", 2200, 300, 6e5), toggle("playMusic", "播放背景音效", true)],
    giftRules: true
  },
  {
    id: "gift-screen",
    name: "礼物飘屏",
    description: "在直播画面中推送礼物飘屏",
    icon: "Tickets",
    accent: "#111111",
    badge: "组件",
    fields: [text("imagePath", "飘屏素材", "images/平底锅.png"), number("durationMs", "持续(ms)", 3500, 100, 6e5), number("maxVisible", "最多同时显示", 20, 1, 100)],
    giftRules: true
  },
  {
    id: "lottery",
    name: "大转盘",
    description: "转盘按照概率抽取奖项",
    icon: "DataAnalysis",
    accent: "#b65ce5",
    badge: "组件",
    fields: [text("pool", "奖项池", "一等奖, 二等奖, 谢谢参与"), text("weights", "权重", "1, 10, 89"), number("durationMs", "动画时长(ms)", 1700, 300, 6e5)],
    giftRules: true
  },
  {
    id: "counter",
    name: "计数器",
    description: "统计数量，计算数量",
    icon: "DataAnalysis",
    accent: "#1bbd93",
    badge: "组件",
    fields: [text("displayContent", "显示标题", "礼物计数"), number("initialValue", "初始值", 0, 0, 999999), { ...number("step", "计数增量", 1, 1, 9999), giftOnly: true }],
    giftRules: true
  },
  {
    id: "gift-pool",
    name: "礼物咖",
    description: "礼物贴纸，玩法菜单设置",
    icon: "Tickets",
    accent: "#f3ae4a",
    badge: "组件",
    fields: [text("pool", "贴纸池", "啤酒, 小心心, 平底锅"), number("durationMs", "显示(ms)", 3500, 100, 6e5), toggle("randomize", "随机素材", true)],
    giftRules: true
  },
  {
    id: "screen-lock",
    name: "屏幕锁键",
    description: "整蛊主播的屏幕锁，需键盘按空格键解锁",
    icon: "Lock",
    accent: "#3978ef",
    badge: "组件",
    fields: [text("displayContent", "解锁提示", "123", "例如：按空格键解锁"), color("lockColor", "锁键颜色", "#e53935"), text("unlockKey", "解锁按键", "SPACE")],
    giftRules: true
  },
  {
    id: "mosquito-slap",
    name: "拍蚊子",
    description: "用于显示拍蚊子玩法",
    icon: "Pointer",
    accent: "#3987ed",
    badge: "动作组件",
    fields: [text("imagePath", "蚊子素材", "idle.png"), number("durationMs", "游戏时长(ms)", 2e4, 1e3, 6e5), number("score", "命中得分", 1, 1, 999)],
    giftRules: true
  }
];
function createDefaultFeatureSettings(input) {
  const settings2 = {};
  for (const feature of FEATURE_DEFINITIONS) {
    const override = input?.[feature.id];
    const values = Object.fromEntries(feature.fields.map((field) => [field.key, field.defaultValue]));
    if (feature.id === "electronic-woodfish" && override?.values?.meritPerGift === void 0 && typeof override?.values?.meritPerClick === "number") {
      values.meritPerGift = override.values.meritPerClick;
    }
    settings2[feature.id] = {
      enabled: override?.enabled ?? false,
      values: { ...values, ...override?.values ?? {} },
      giftRules: override?.giftRules ? override.giftRules.map((rule) => ({ ...rule, values: { ...rule.values ?? {} } })) : defaultGiftRules(feature)
    };
  }
  return settings2;
}
function defaultGiftRules(feature) {
  if (!feature.giftRules) return [];
  return [{ id: `${feature.id}-default-rule`, giftName: "啤酒", action: feature.id === "frying-pan" ? "减少" : "增加", enabled: true, values: {} }];
}
function eventContent(event) {
  if (event.kind === "gift") return event.gift?.name ?? "";
  if (event.kind === "chat") return event.text ?? "";
  return event.text ?? event.gift?.name ?? "";
}
function matchRule(rule, event) {
  if (!rule.enabled || !rule.trigger.kinds.includes(event.kind)) return false;
  if (rule.trigger.source?.length && !rule.trigger.source.includes(event.source)) return false;
  const userName = event.user?.name ?? "";
  if (rule.trigger.users?.length && !rule.trigger.users.includes(userName)) return false;
  if (rule.trigger.minCount && (event.gift?.count ?? event.count ?? 0) < rule.trigger.minCount) return false;
  if (rule.trigger.giftNames?.length && !rule.trigger.giftNames.includes(event.gift?.name ?? "")) return false;
  const keywords = rule.trigger.keywords?.filter(Boolean) ?? [];
  if (!keywords.length) return true;
  const content = eventContent(event);
  const mode = rule.trigger.keywordMode ?? "contains";
  return keywords.some((keyword) => {
    if (mode === "exact") return content === keyword;
    if (mode === "regex") {
      try {
        return new RegExp(keyword, "i").test(content);
      } catch {
        return false;
      }
    }
    return content.toLocaleLowerCase().includes(keyword.toLocaleLowerCase());
  });
}
class RuleEngine {
  constructor(context, options = {}) {
    this.context = context;
    this.random = options.random ?? Math.random;
    this.now = options.now ?? Date.now;
  }
  context;
  nextAllowed = /* @__PURE__ */ new Map();
  active = /* @__PURE__ */ new Map();
  queues = /* @__PURE__ */ new Map();
  random;
  now;
  async process(event, rules) {
    const candidates = rules.filter((rule) => matchRule(rule, event)).sort((a, b) => b.priority - a.priority || (a.updatedAt ?? 0) - (b.updatedAt ?? 0));
    for (const rule of candidates) {
      const skip = this.checkSkip(rule);
      if (skip) {
        this.context.log("debug", `[rule:${rule.name}] skipped: ${skip}`);
        continue;
      }
      const result = await this.schedule(rule, event);
      if (result.result !== "skipped") return result;
    }
    return { result: "none", reason: candidates.length ? "所有匹配规则均被跳过" : void 0 };
  }
  checkSkip(rule) {
    const now = this.now();
    const next = this.nextAllowed.get(rule.id) ?? 0;
    if (now < next) return `冷却中，还需 ${next - now}ms`;
    if (rule.probability !== void 0 && this.random() > rule.probability) return "概率未命中";
    if (rule.concurrency === "exclusive" && (this.active.get(rule.id) ?? 0) > 0) return "互斥动作正在执行";
    return void 0;
  }
  schedule(rule, event) {
    const run = async () => {
      const now = this.now();
      if (rule.cooldownMs) this.nextAllowed.set(rule.id, now + rule.cooldownMs);
      this.active.set(rule.id, (this.active.get(rule.id) ?? 0) + 1);
      this.context.log("info", `[rule:${rule.name}] matched priority=${rule.priority}`);
      try {
        for (const action of rule.actions) {
          const repeats = Math.max(1, action.repeat ?? 1);
          for (let i = 0; i < repeats; i += 1) {
            if (action.delayMs) await delay(action.delayMs);
            const result = await this.context.executeAction(action, event, rule);
            if (!result.ok) {
              this.context.log("warn", `[rule:${rule.name}] action failed`, result.message);
              return { ruleId: rule.id, ruleName: rule.name, result: "failed", reason: result.message };
            }
          }
        }
        return { ruleId: rule.id, ruleName: rule.name, result: "ok" };
      } catch (error) {
        const reason = error instanceof Error ? error.message : String(error);
        this.context.log("error", `[rule:${rule.name}] action error`, reason);
        return { ruleId: rule.id, ruleName: rule.name, result: "failed", reason };
      } finally {
        this.active.set(rule.id, Math.max(0, (this.active.get(rule.id) ?? 1) - 1));
      }
    };
    if (rule.concurrency === "queue") {
      const previous = this.queues.get(rule.id) ?? Promise.resolve();
      const current = previous.then(run, run);
      this.queues.set(rule.id, current.finally(() => {
        if (this.queues.get(rule.id) === current) this.queues.delete(rule.id);
      }));
      return current;
    }
    if (rule.concurrency === "replace") {
      this.context.log("debug", `[rule:${rule.name}] replace policy: start latest action`);
    }
    return run();
  }
}
function delay(ms) {
  return new Promise((resolve2) => setTimeout(resolve2, ms));
}
class ActionExecutor {
  constructor(windows2, logger2) {
    this.windows = windows2;
    this.logger = logger2;
  }
  windows;
  logger;
  async execute(action, event, rule) {
    switch (action.kind) {
      case "video":
        this.logger.info(`[action:video] lane=${action.lane ?? 1} started path=${action.path}`);
        await this.windows.playVideo({ path: action.path, lane: action.lane, durationMs: action.durationMs, loop: action.loop, chroma: action.chroma });
        return { ok: true };
      case "drop":
        this.logger.info(`[action:drop] count=${action.count ?? 1} image=${action.image}`);
        await this.windows.drop({ image: action.image, count: action.count, gravity: action.gravity, bounce: action.bounce, durationMs: action.durationMs });
        return { ok: true };
      case "slot":
        this.logger.info("[action:slot] start weighted result");
        await this.windows.slot({ theme: action.theme, pool: action.pool, weights: action.weights });
        return { ok: true };
      case "audio":
        this.logger.info(`[action:audio] path=${action.path} interrupt=${Boolean(action.interrupt)}`);
        await this.windows.playAudio({ path: action.path, volume: action.volume, interrupt: action.interrupt, loop: action.loop });
        return { ok: true };
      case "key":
        this.logger.info(`[action:key] target=${action.target?.processName ?? action.target?.title ?? "foreground"} steps=${action.steps.length} event=${event.kind}`);
        return { ok: true, message: "本地演示已记录；Windows 原生 SendInput 适配可在 input-helper 中替换" };
      case "mouse":
        this.logger.info(`[action:mouse] target=${action.target?.processName ?? action.target?.title ?? "foreground"} steps=${action.steps.length}`);
        return { ok: true, message: "本地演示已记录；Windows 坐标/查图适配可在 input-helper 中替换" };
      case "serial":
        this.logger.info(`[action:serial] ${action.port} pulse=${action.pulseMs}ms bytes=${action.onBytes.join(",")}`);
        return { ok: true, message: "虚拟串口模式：已记录脉冲，未向物理端口写入" };
      case "obs":
        this.logger.info(`[action:obs] command=${action.command}`);
        return { ok: true, message: "OBS 未连接时降级为日志动作" };
    }
  }
}
class ConnectorManager {
  statusValue = {
    platform: "simulator",
    state: "disconnected",
    mode: "simulator",
    reconnects: 0,
    dropped: 0
  };
  eventListeners = /* @__PURE__ */ new Set();
  statusListeners = /* @__PURE__ */ new Set();
  dedupe = /* @__PURE__ */ new Map();
  buffered = /* @__PURE__ */ new Map();
  onEvent(listener) {
    this.eventListeners.add(listener);
    return () => this.eventListeners.delete(listener);
  }
  onStatus(listener) {
    this.statusListeners.add(listener);
    return () => this.statusListeners.delete(listener);
  }
  async connect(platform, roomId) {
    this.setStatus({ platform, roomId, state: "connecting", mode: "simulator", reconnects: this.statusValue.reconnects, dropped: 0 });
    await delay(120);
    this.setStatus({ platform, roomId, state: "connected", mode: "simulator", reconnects: 0, dropped: 0, lastEventAt: void 0 });
    return this.statusValue;
  }
  async disconnect() {
    this.setStatus({ ...this.statusValue, state: "disconnected" });
    return this.statusValue;
  }
  getStatus() {
    return { ...this.statusValue };
  }
  async publish(input) {
    const event = {
      id: input.id ?? randomUUID(),
      source: input.source ?? this.statusValue.platform,
      roomId: input.roomId ?? this.statusValue.roomId,
      kind: input.kind,
      user: input.user ?? { id: "sim-user", name: "模拟观众" },
      gift: input.gift,
      text: input.text,
      count: input.count,
      timestamp: input.timestamp ?? Date.now(),
      raw: input.raw
    };
    const key = dedupeKey(event);
    const lastSeen = this.dedupe.get(key) ?? 0;
    if (event.timestamp - lastSeen < 3e3) return event;
    this.dedupe.set(key, event.timestamp);
    this.pruneDedupe(event.timestamp);
    const platform = event.source;
    const queue = this.buffered.get(platform) ?? [];
    if (queue.length >= 1e3) {
      if (event.kind === "gift") {
        const index = queue.findIndex((item) => item.kind !== "gift");
        if (index >= 0) queue.splice(index, 1);
        else {
          this.statusValue.dropped += 1;
          return event;
        }
      } else {
        this.statusValue.dropped += 1;
        return event;
      }
    }
    queue.push(event);
    this.buffered.set(platform, queue);
    const next = queue.shift();
    if (next) {
      this.statusValue.lastEventAt = next.timestamp;
      for (const listener of this.eventListeners) listener(next);
    }
    this.emitStatus();
    return event;
  }
  setStatus(status) {
    this.statusValue = { ...status };
    this.emitStatus();
  }
  emitStatus() {
    for (const listener of this.statusListeners) listener(this.getStatus());
  }
  pruneDedupe(now) {
    for (const [key, ts] of this.dedupe) if (now - ts > 3e4) this.dedupe.delete(key);
  }
}
function dedupeKey(event) {
  return [event.source, event.user?.id ?? event.user?.name ?? "", event.kind, event.gift?.name ?? "", event.text ?? "", Math.floor(event.timestamp / 1e3)].join("|");
}
class AppLogger {
  constructor(logDir) {
    this.logDir = logDir;
  }
  logDir;
  listeners = /* @__PURE__ */ new Set();
  nextId = Date.now() * 1e3;
  on(listener) {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }
  write(level, category, message, detail) {
    const entry = {
      id: this.nextId++,
      level,
      category,
      message: redact(message),
      detail: detail ? redact(detail) : void 0,
      ts: Date.now()
    };
    for (const listener of this.listeners) listener(entry);
    void this.persist(entry);
    return entry;
  }
  info(message, detail) {
    return this.write("info", "app", message, detail);
  }
  warn(message, detail) {
    return this.write("warn", "app", message, detail);
  }
  error(message, detail) {
    return this.write("error", "app", message, detail);
  }
  debug(message, detail) {
    return this.write("debug", "app", message, detail);
  }
  async persist(entry) {
    try {
      await mkdir(this.logDir, { recursive: true });
      await appendFile(join(this.logDir, "app.log"), `${JSON.stringify(entry)}
`, "utf8");
    } catch {
    }
  }
}
function redact(value) {
  return value.replace(/(cookie|authorization|token|password|kami|card|卡密)\s*[:=]\s*[^\s,;]+/gi, "$1=***").replace(/Bearer\s+[A-Za-z0-9._~-]+/gi, "Bearer ***");
}
const localRequire = createRequire(import.meta.url);
class SqliteStore {
  db;
  async init(filePath) {
    await mkdir(dirname(filePath), { recursive: true });
    const wasmPath = this.resolveWasmPath();
    const SQL = await initSqlJs({ locateFile: () => wasmPath });
    if (existsSync(filePath)) {
      const bytes = await readFile(filePath);
      this.db = new SQL.Database(new Uint8Array(bytes));
    } else {
      this.db = new SQL.Database();
    }
    this.db.run("PRAGMA journal_mode = WAL;");
    this.db.run(`
      CREATE TABLE IF NOT EXISTS rules (
        id TEXT PRIMARY KEY,
        json TEXT NOT NULL,
        updated_at INTEGER NOT NULL
      );
      CREATE TABLE IF NOT EXISTS danmaku_records (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        source TEXT NOT NULL,
        room_id TEXT,
        kind TEXT NOT NULL,
        user_id TEXT,
        user_name TEXT,
        text TEXT,
        gift_name TEXT,
        gift_count INTEGER,
        gift_value REAL,
        repeat_count INTEGER,
        ts INTEGER NOT NULL,
        created_at INTEGER NOT NULL,
        matched_rule_id TEXT,
        matched_rule_name TEXT,
        action_result TEXT NOT NULL DEFAULT 'none',
        fail_reason TEXT,
        raw_json TEXT
      );
      CREATE INDEX IF NOT EXISTS idx_danmaku_ts ON danmaku_records(ts);
      CREATE INDEX IF NOT EXISTS idx_danmaku_source_ts ON danmaku_records(source, ts);
      CREATE INDEX IF NOT EXISTS idx_danmaku_kind_ts ON danmaku_records(kind, ts);
      CREATE INDEX IF NOT EXISTS idx_danmaku_user ON danmaku_records(user_name);
      CREATE TABLE IF NOT EXISTS logs (
        id INTEGER PRIMARY KEY,
        level TEXT NOT NULL,
        category TEXT NOT NULL,
        message TEXT NOT NULL,
        detail TEXT,
        ts INTEGER NOT NULL
      );
      CREATE TABLE IF NOT EXISTS settings (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL
      );
    `);
    await this.persist(filePath);
  }
  async listRules() {
    return this.all("SELECT json FROM rules ORDER BY json_extract(json, '$.priority') DESC, updated_at ASC").map((row) => JSON.parse(String(row.json)));
  }
  async saveRule(rule) {
    const updated = { ...rule, updatedAt: Date.now(), createdAt: rule.createdAt ?? Date.now() };
    this.run("INSERT OR REPLACE INTO rules(id, json, updated_at) VALUES (?, ?, ?)", [updated.id, JSON.stringify(updated), updated.updatedAt]);
    return updated;
  }
  async removeRule(id) {
    this.run("DELETE FROM rules WHERE id = ?", [id]);
  }
  async clearRules() {
    this.db.run("DELETE FROM rules");
  }
  insertRecord(record) {
    this.run(
      `INSERT INTO danmaku_records
        (source, room_id, kind, user_id, user_name, text, gift_name, gift_count, gift_value, repeat_count, ts, created_at, matched_rule_id, matched_rule_name, action_result, fail_reason, raw_json)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      [
        record.source,
        record.roomId ?? null,
        record.kind,
        record.userId ?? null,
        record.userName ?? null,
        record.text ?? null,
        record.giftName ?? null,
        record.giftCount ?? null,
        record.giftValue ?? null,
        record.repeatCount ?? null,
        record.ts,
        record.createdAt,
        record.matchedRuleId ?? null,
        record.matchedRuleName ?? null,
        record.actionResult,
        record.failReason ?? null,
        record.rawJson ?? null
      ]
    );
    const row = this.all("SELECT last_insert_rowid() AS id")[0];
    return Number(row?.id ?? 0);
  }
  updateRecordResult(id, result) {
    this.run("UPDATE danmaku_records SET matched_rule_id = ?, matched_rule_name = ?, action_result = ?, fail_reason = ? WHERE id = ?", [
      result.matchedRuleId ?? null,
      result.matchedRuleName ?? null,
      result.actionResult,
      result.failReason ?? null,
      id
    ]);
  }
  queryRecords(filter = {}) {
    const { where, params } = buildRecordWhere(filter);
    const limit = Math.min(Math.max(filter.limit ?? 100, 1), 500);
    const offset = Math.max(filter.offset ?? 0, 0);
    return this.all(
      `SELECT id, source, room_id AS roomId, kind, user_id AS userId, user_name AS userName,
        text, gift_name AS giftName, gift_count AS giftCount, gift_value AS giftValue,
        repeat_count AS repeatCount, ts, created_at AS createdAt, matched_rule_id AS matchedRuleId,
        matched_rule_name AS matchedRuleName, action_result AS actionResult, fail_reason AS failReason,
        raw_json AS rawJson FROM danmaku_records ${where} ORDER BY ts DESC, id DESC LIMIT ? OFFSET ?`,
      [...params, limit, offset]
    ).map((row) => normalizeRecord(row));
  }
  countRecords(filter = {}) {
    const { where, params } = buildRecordWhere(filter);
    const row = this.all(`SELECT COUNT(*) AS count FROM danmaku_records ${where}`, params)[0];
    return Number(row?.count ?? 0);
  }
  clearRecords(range = {}) {
    const conditions = [];
    const params = [];
    if (range.from !== void 0) {
      conditions.push("ts >= ?");
      params.push(range.from);
    }
    if (range.to !== void 0) {
      conditions.push("ts <= ?");
      params.push(range.to);
    }
    const before = this.countRecords();
    this.run(`DELETE FROM danmaku_records${conditions.length ? ` WHERE ${conditions.join(" AND ")}` : ""}`, params);
    return before - this.countRecords();
  }
  cleanupRetention(days, maxRows) {
    const cutoff = Date.now() - Math.max(days, 1) * 24 * 60 * 60 * 1e3;
    const before = this.countRecords();
    this.run("DELETE FROM danmaku_records WHERE ts < ?", [cutoff]);
    const excess = Math.max(0, this.countRecords() - Math.max(maxRows, 1));
    if (excess) this.run("DELETE FROM danmaku_records WHERE id IN (SELECT id FROM danmaku_records ORDER BY ts ASC, id ASC LIMIT ?)", [excess]);
    return before - this.countRecords();
  }
  saveLog(entry) {
    this.run("INSERT OR REPLACE INTO logs(id, level, category, message, detail, ts) VALUES (?, ?, ?, ?, ?, ?)", [entry.id, entry.level, entry.category, entry.message, entry.detail ?? null, entry.ts]);
  }
  listLogs(limit = 100) {
    return this.all("SELECT id, level, category, message, detail, ts FROM logs ORDER BY id DESC LIMIT ?", [Math.min(Math.max(limit, 1), 500)]);
  }
  getSetting(key, fallback) {
    const row = this.all("SELECT value FROM settings WHERE key = ?", [key])[0];
    if (!row) return fallback;
    try {
      return JSON.parse(String(row.value));
    } catch {
      return fallback;
    }
  }
  setSetting(key, value) {
    this.run("INSERT OR REPLACE INTO settings(key, value) VALUES (?, ?)", [key, JSON.stringify(value)]);
  }
  async persist(filePath) {
    if (!this.db) return;
    const bytes = this.db.export();
    const tempPath = `${filePath}.tmp`;
    await writeFile(tempPath, bytes);
    await rename(tempPath, filePath);
  }
  all(sql, params = []) {
    const statement = this.db.prepare(sql);
    statement.bind(params);
    const rows = [];
    while (statement.step()) rows.push(statement.getAsObject());
    statement.free();
    return rows;
  }
  run(sql, params = []) {
    const statement = this.db.prepare(sql);
    statement.run(params);
    statement.free();
  }
  resolveWasmPath() {
    try {
      const resolved = localRequire.resolve("sql.js/dist/sql-wasm.wasm");
      return resolved;
    } catch {
      return join(process.resourcesPath, "sql.js", "sql-wasm.wasm");
    }
  }
}
function buildRecordWhere(filter) {
  const clauses = [];
  const params = [];
  if (filter.from !== void 0) {
    clauses.push("ts >= ?");
    params.push(filter.from);
  }
  if (filter.to !== void 0) {
    clauses.push("ts <= ?");
    params.push(filter.to);
  }
  if (filter.source) {
    clauses.push("source = ?");
    params.push(filter.source);
  }
  if (filter.kind) {
    clauses.push("kind = ?");
    params.push(filter.kind);
  }
  if (filter.userName) {
    clauses.push("user_name LIKE ?");
    params.push(`%${filter.userName}%`);
  }
  if (filter.keyword) {
    clauses.push("(text LIKE ? OR gift_name LIKE ?)");
    params.push(`%${filter.keyword}%`, `%${filter.keyword}%`);
  }
  if (filter.actionResult) {
    clauses.push("action_result = ?");
    params.push(filter.actionResult);
  }
  if (filter.matched === "yes") clauses.push("matched_rule_id IS NOT NULL");
  if (filter.matched === "no") clauses.push("matched_rule_id IS NULL");
  return { where: clauses.length ? `WHERE ${clauses.join(" AND ")}` : "", params };
}
function normalizeRecord(row) {
  return {
    id: Number(row.id),
    source: String(row.source),
    roomId: nullableString(row.roomId),
    kind: row.kind,
    userId: nullableString(row.userId),
    userName: nullableString(row.userName),
    text: nullableString(row.text),
    giftName: nullableString(row.giftName),
    giftCount: nullableNumber(row.giftCount),
    giftValue: nullableNumber(row.giftValue),
    repeatCount: nullableNumber(row.repeatCount),
    ts: Number(row.ts),
    createdAt: Number(row.createdAt),
    matchedRuleId: nullableString(row.matchedRuleId),
    matchedRuleName: nullableString(row.matchedRuleName),
    actionResult: row.actionResult ?? "none",
    failReason: nullableString(row.failReason),
    rawJson: nullableString(row.rawJson)
  };
}
function nullableString(value) {
  return value === null || value === void 0 ? void 0 : String(value);
}
function nullableNumber(value) {
  return value === null || value === void 0 ? void 0 : Number(value);
}
class WindowManager {
  constructor(preloadPath) {
    this.preloadPath = preloadPath;
  }
  preloadPath;
  mainWindow;
  greenWindow;
  slotWindow;
  audioWindow;
  quitting = false;
  loadPromises = /* @__PURE__ */ new WeakMap();
  listeners = /* @__PURE__ */ new Set();
  statusListeners = /* @__PURE__ */ new Set();
  slotWidgets = /* @__PURE__ */ new Map();
  modes = { green: "green", slot: "landscape-16-9" };
  settings = {
    green: { visible: false, alwaysOnTop: true, opacity: 1, backgroundTransparent: false, width: 960, height: 540, laneCount: 3, background: "#00ff00" },
    slot: { visible: false, alwaysOnTop: true, opacity: 1, backgroundTransparent: true, width: 960, height: 540, laneCount: 1, background: "#000000" }
  };
  onMessage(listener) {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }
  onStatus(listener) {
    this.statusListeners.add(listener);
    return () => this.statusListeners.delete(listener);
  }
  attachMainWindow(window) {
    this.mainWindow = window;
  }
  markQuitting() {
    this.quitting = true;
  }
  getSettings(type) {
    return { ...this.settings[type] };
  }
  hasEventWidget(featureId) {
    const payload = this.slotWidgets.get(featureId);
    return Boolean(payload && payload.data?.preview !== true);
  }
  getStatus() {
    return ["green", "slot"].map((type) => this.statusFor(type));
  }
  updateSettings(type, patch) {
    this.settings[type] = { ...this.settings[type], ...patch };
    const target = type === "green" ? this.greenWindow : this.slotWindow;
    if (target && !target.isDestroyed()) {
      this.applySettings(type, target);
    }
    this.emitStatus(type);
    return this.getSettings(type);
  }
  async open(type) {
    const existing = type === "green" ? this.greenWindow : this.slotWindow;
    const created = !existing || existing.isDestroyed();
    const window = this.ensureOverlay(type);
    await this.waitReady(window);
    this.settings[type].visible = true;
    window.showInactive();
    this.applySettings(type, window);
    await window.webContents.executeJavaScript('window.dispatchEvent(new Event("overlay-ready"))').catch(() => void 0);
    if (type === "slot" && created) {
      for (const payload of this.slotWidgets.values()) this.send("slot", "component-widget", payload);
    }
    this.emitStatus(type);
  }
  async close(type) {
    this.settings[type].visible = false;
    const target = type === "green" ? this.greenWindow : this.slotWindow;
    if (target && !target.isDestroyed()) target.hide();
    this.emitStatus(type);
  }
  async setMode(type, mode) {
    const window = this.ensureOverlay(type);
    await this.waitReady(window);
    this.modes[type] = mode;
    if (mode === "fullscreen") {
      window.setFullScreen(true);
    } else {
      if (window.isFullScreen()) window.setFullScreen(false);
      const size = this.sizeFor(type, mode);
      this.settings[type] = { ...this.settings[type], visible: true, width: size.width, height: size.height };
      window.setSize(size.width, size.height);
      window.center();
      this.applySettings(type, window);
    }
    this.settings[type].visible = true;
    window.showInactive();
    this.emitStatus(type);
    return this.statusFor(type);
  }
  async toggleOpacity(type) {
    const window = this.ensureOverlay(type);
    await this.waitReady(window);
    if (type === "green") {
      const opacity = this.settings.green.opacity <= 0.3 ? 1 : 0.25;
      this.settings.green.opacity = opacity;
      if (!window.isDestroyed()) window.setOpacity(clampOpacity(opacity));
    } else {
      this.settings.slot.backgroundTransparent = !this.slotBackgroundTransparent();
      if (!window.isDestroyed()) window.setBackgroundColor(this.nativeBackground("slot", this.settings.slot.background));
      this.notifySlotBackground(window);
    }
    this.emitStatus(type);
    return this.statusFor(type);
  }
  async playVideo(payload) {
    await this.open("green");
    this.send("green", "play-video", { ...payload, path: toMediaUrl(payload.path) });
  }
  async drop(payload) {
    await this.open("green");
    this.send("green", "drop", { ...payload, image: toMediaUrl(payload.image) });
  }
  async slot(payload) {
    await this.open("slot");
    this.send("slot", "slot-start", { ...payload, images: payload.images?.map(toMediaUrl), audioPath: payload.audioPath ? toMediaUrl(payload.audioPath) : void 0 });
  }
  async widget(payload) {
    const slotWindowWasAbsent = !this.slotWindow || this.slotWindow.isDestroyed();
    if (payload.kind !== "speech") {
      this.slotWidgets.set(payload.featureId, {
        ...payload,
        values: { ...payload.values },
        data: payload.data ? { ...payload.data } : void 0
      });
    }
    await this.open("slot");
    if (slotWindowWasAbsent && payload.kind !== "speech") return;
    this.send("slot", "component-widget", payload);
  }
  removeWidget(featureId) {
    this.slotWidgets.delete(featureId);
    const window = this.slotWindow;
    if (window && !window.isDestroyed()) this.send("slot", "component-remove", { featureId });
  }
  async playAudio(payload) {
    const window = this.ensureAudio();
    await this.waitReady(window);
    this.sendAudio("play-audio", { ...payload, path: toMediaUrl(payload.path) });
    if (window.isDestroyed()) this.audioWindow = void 0;
  }
  async stopAudio() {
    if (this.audioWindow && !this.audioWindow.isDestroyed()) await this.waitReady(this.audioWindow);
    this.sendAudio("stop-audio", {});
  }
  destroyAll() {
    for (const window of [this.greenWindow, this.slotWindow, this.audioWindow]) {
      if (window && !window.isDestroyed()) window.destroy();
    }
    this.greenWindow = void 0;
    this.slotWindow = void 0;
    this.audioWindow = void 0;
  }
  ensureOverlay(type) {
    const existing = type === "green" ? this.greenWindow : this.slotWindow;
    if (existing && !existing.isDestroyed()) return existing;
    const value = this.settings[type];
    const display = screen.getPrimaryDisplay().workAreaSize;
    const window = new BrowserWindow({
      width: value.width,
      height: value.height,
      x: Math.max(0, Math.round((display.width - value.width) / 2)),
      y: Math.max(0, Math.round((display.height - value.height) / 2)),
      title: this.titleFor(type),
      frame: true,
      // 两个 Overlay 都是普通窗口（系统原生标题栏，可拖动/可拉边）；组件窗口的“透明”
      // 由 applySlotColorKey 在运行时给窗口加分层 + 抠像黑实现，和原版 yapp 完全一致。
      transparent: false,
      backgroundColor: this.nativeBackground(type, value.background),
      resizable: true,
      minimizable: false,
      maximizable: false,
      alwaysOnTop: value.alwaysOnTop,
      skipTaskbar: false,
      show: false,
      opacity: type === "slot" ? 1 : clampOpacity(value.opacity),
      autoHideMenuBar: true,
      webPreferences: this.webPreferences()
    });
    window.setTitle(this.titleFor(type));
    window.setMenuBarVisibility(false);
    window.setSkipTaskbar(false);
    window.webContents.on("page-title-updated", (event) => {
      event.preventDefault();
      window.setTitle(this.titleFor(type));
    });
    window.on("closed", () => {
      this.settings[type].visible = false;
      if (type === "green") this.greenWindow = void 0;
      else this.slotWindow = void 0;
      this.emit({ target: type, type: "window-closed", payload: {} });
      this.emitStatus(type);
    });
    window.on("resize", () => this.emitStatus(type));
    window.on("move", () => this.emitStatus(type));
    window.on("enter-full-screen", () => this.emitStatus(type));
    window.on("leave-full-screen", () => this.emitStatus(type));
    this.trackLoad(window, type === "green" ? "overlay-green" : "overlay-slot");
    if (type === "green") this.greenWindow = window;
    else this.slotWindow = window;
    return window;
  }
  ensureAudio() {
    if (this.audioWindow && !this.audioWindow.isDestroyed()) return this.audioWindow;
    const window = new BrowserWindow({
      width: 2,
      height: 2,
      show: false,
      webPreferences: this.webPreferences()
    });
    this.trackLoad(window, "overlay-audio");
    this.audioWindow = window;
    return window;
  }
  async load(window, hash) {
    const rendererUrl = process.env.ELECTRON_RENDERER_URL;
    if (rendererUrl) await window.loadURL(`${rendererUrl}#/${hash}`);
    else await window.loadFile(join(__dirname, "../renderer/index.html"), { hash: `/${hash}` });
  }
  trackLoad(window, hash) {
    const loadPromise = this.load(window, hash).catch(() => void 0);
    this.loadPromises.set(window, loadPromise);
  }
  async waitReady(window) {
    await this.loadPromises.get(window);
  }
  webPreferences() {
    return { preload: this.preloadPath, sandbox: true, contextIsolation: true, nodeIntegration: false };
  }
  applySettings(type, window) {
    const value = this.settings[type];
    window.setAlwaysOnTop(value.alwaysOnTop);
    window.setOpacity(type === "slot" ? 1 : clampOpacity(value.opacity));
    if (!window.isFullScreen()) window.setSize(Math.max(320, value.width), Math.max(240, value.height));
    window.setBackgroundColor(this.nativeBackground(type, value.background));
    if (type === "slot") this.notifySlotBackground(window);
    if (value.visible) window.showInactive();
    else window.hide();
  }
  /** 组件窗口底板是否完全透明（默认透明；老配置缺该字段时按透明处理）。 */
  slotBackgroundTransparent() {
    return this.settings.slot.backgroundTransparent !== false;
  }
  notifySlotBackground(window) {
    if (window.isDestroyed()) return;
    window.webContents.send("overlay:message", { target: "slot", type: "background-mode", payload: { transparent: this.slotBackgroundTransparent() } });
  }
  sizeFor(type, mode) {
    if (mode === "green") return { width: this.settings[type].width || 960, height: this.settings[type].height || 540 };
    if (mode === "landscape-4-3") return { width: 800, height: 600 };
    if (mode === "portrait-9-16") return { width: 540, height: 960 };
    return { width: 960, height: 540 };
  }
  statusFor(type) {
    const window = type === "green" ? this.greenWindow : this.slotWindow;
    const bounds = window && !window.isDestroyed() ? window.getBounds() : { width: this.settings[type].width, height: this.settings[type].height };
    return {
      type,
      role: type === "green" ? "ylm" : "yapp",
      title: this.titleFor(type),
      visible: Boolean(window && !window.isDestroyed() && window.isVisible()),
      width: bounds.width,
      height: bounds.height,
      mode: this.modes[type],
      fullscreen: Boolean(window && !window.isDestroyed() && window.isFullScreen()),
      opacity: this.settings[type].opacity,
      backgroundTransparent: type === "slot" ? this.slotBackgroundTransparent() : false
    };
  }
  emitStatus(type) {
    const status = this.statusFor(type);
    for (const listener of this.statusListeners) listener(status);
  }
  titleFor(type) {
    return type === "green" ? "AKA直播 - 绿幕窗口 【禁止最小化】（按Tab键可以管理视频列表）" : "AKA直播 - 组件窗口 【禁止最小化】 快捷键切换透明度 Ctrl + F1";
  }
  nativeBackground(type, color2) {
    if (type === "green") return color2 === "transparent" ? "#00ff00" : color2;
    return this.slotBackgroundTransparent() ? "#000000" : "#080d18";
  }
  send(target, type, payload) {
    const window = target === "green" ? this.greenWindow : this.slotWindow;
    if (window && !window.isDestroyed()) window.webContents.send("overlay:message", { target, type, payload });
    this.emit({ target, type, payload });
  }
  sendAudio(type, payload) {
    if (this.audioWindow && !this.audioWindow.isDestroyed()) this.audioWindow.webContents.send("audio:message", { type, payload });
  }
  emit(message) {
    for (const listener of this.listeners) listener(message);
  }
}
function toMediaUrl(filePath) {
  if (/^(file|https?):\/\//i.test(filePath)) return filePath;
  return pathToFileURL(filePath).toString();
}
function clampOpacity(value) {
  return Math.min(1, Math.max(0.1, value));
}
class ObsWebSocketClient {
  socket;
  connected = false;
  url = "";
  password = "";
  message = "OBS 未连接";
  virtualCameraActive = false;
  identified;
  pending = /* @__PURE__ */ new Map();
  async connect(url, password = "") {
    if (this.connected && this.url === url) return this.status();
    this.disconnect();
    this.url = url.trim();
    this.password = password;
    this.message = "正在连接 OBS WebSocket…";
    return new Promise((resolve2, reject) => {
      const timeout = setTimeout(() => fail(new Error("连接 OBS 超时，请确认 OBS WebSocket 已启用且地址、端口正确。")), 8e3);
      const fail = (error) => {
        clearTimeout(timeout);
        this.connected = false;
        this.message = error.message;
        this.socket?.close();
        reject(error);
      };
      try {
        const socket = new WebSocket(this.url, "obswebsocket.json");
        this.socket = socket;
        this.identified = { resolve: () => {
          clearTimeout(timeout);
          this.connected = true;
          this.message = "OBS WebSocket 已连接";
          resolve2(this.status());
        }, reject: fail, timeout };
        socket.onmessage = (event) => this.receive(String(event.data));
        socket.onerror = () => {
          if (!this.connected) fail(new Error("无法连接 OBS WebSocket，请检查 OBS 是否已启动和密码是否正确。"));
        };
        socket.onclose = () => {
          if (!this.connected) fail(new Error("OBS WebSocket 已关闭"));
          this.connected = false;
          this.virtualCameraActive = false;
          this.message = "OBS WebSocket 已断开";
          for (const pending of this.pending.values()) {
            clearTimeout(pending.timeout);
            pending.reject(new Error(this.message));
          }
          this.pending.clear();
        };
      } catch (error) {
        fail(error instanceof Error ? error : new Error(String(error)));
      }
    });
  }
  async command(requestType, requestData = {}) {
    if (!this.connected || !this.socket || this.socket.readyState !== WebSocket.OPEN) throw new Error("请先连接 OBS WebSocket");
    const requestId = randomUUID();
    return new Promise((resolve2, reject) => {
      const timeout = setTimeout(() => {
        this.pending.delete(requestId);
        reject(new Error(`OBS 请求 ${requestType} 超时`));
      }, 8e3);
      this.pending.set(requestId, { resolve: resolve2, reject, timeout });
      this.socket?.send(JSON.stringify({ op: 6, d: { requestType, requestId, requestData } }));
    });
  }
  async startVirtualCamera() {
    try {
      await this.command("StartVirtualCam");
    } catch (error) {
      if (!(error instanceof Error) || !/already running|output is active|already active/i.test(error.message)) throw error;
    }
    this.virtualCameraActive = true;
    this.message = "OBS 虚拟摄像头已启动（输出当前 OBS 场景）";
    return this.status();
  }
  async stopVirtualCamera() {
    try {
      await this.command("StopVirtualCam");
    } catch (error) {
      if (!(error instanceof Error) || !/not running|not active/i.test(error.message)) throw error;
    }
    this.virtualCameraActive = false;
    this.message = "OBS 虚拟摄像头已停止";
    return this.status();
  }
  disconnect() {
    const socket = this.socket;
    this.socket = void 0;
    this.connected = false;
    this.virtualCameraActive = false;
    if (this.identified) {
      clearTimeout(this.identified.timeout);
      this.identified.reject(new Error("OBS 连接已取消"));
      this.identified = void 0;
    }
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timeout);
      pending.reject(new Error("OBS 连接已关闭"));
    }
    this.pending.clear();
    if (socket && socket.readyState < WebSocket.CLOSING) socket.close();
  }
  status() {
    return { connected: this.connected, url: this.url, message: this.message, virtualCameraActive: this.virtualCameraActive };
  }
  receive(text2) {
    let message;
    try {
      message = JSON.parse(text2);
    } catch {
      return;
    }
    if (message.op === 0) {
      const auth2 = message.d.authentication;
      const identify = { rpcVersion: Number(message.d.rpcVersion) || 1 };
      if (auth2?.salt && auth2.challenge) identify.authentication = createAuthentication(this.password, auth2.salt, auth2.challenge);
      this.socket?.send(JSON.stringify({ op: 1, d: identify }));
      return;
    }
    if (message.op === 2) {
      this.identified?.resolve();
      this.identified = void 0;
      return;
    }
    if (message.op !== 7) return;
    const requestId = String(message.d.requestId ?? "");
    const pending = this.pending.get(requestId);
    if (!pending) return;
    this.pending.delete(requestId);
    clearTimeout(pending.timeout);
    const status = message.d.requestStatus;
    if (!status?.result) pending.reject(new Error(String(status?.comment ?? "OBS 请求失败")));
    else pending.resolve(message.d.responseData ?? {});
  }
}
function createAuthentication(password, salt, challenge) {
  const secret = createHash("sha256").update(`${password}${salt}`).digest("base64");
  return createHash("sha256").update(`${secret}${challenge}`).digest("base64");
}
class LicenseClient {
  userSession = "";
  userServerUrl = "";
  adminToken = "";
  adminServerUrl = "";
  adminExpiresAt = "";
  async login(serverUrl, code, platform, clientId) {
    await this.logout().catch(() => void 0);
    const baseUrl = normalizeServerUrl(serverUrl);
    const result = await this.request(baseUrl, "/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ code, platform, client_id: clientId })
    });
    if (!result.session || !isValidDate(result.expires_at) || !isValidDate(result.session_expires_at) || !isEntitlementList(result.features)) {
      throw new Error("授权服务返回的数据不完整");
    }
    this.userSession = result.session;
    this.userServerUrl = baseUrl;
    return {
      expiresAt: result.expires_at,
      sessionExpiresAt: result.session_expires_at,
      features: result.features,
      safeCodeSet: Boolean(result.safe_code_set)
    };
  }
  async status(serverUrl) {
    const baseUrl = normalizeServerUrl(serverUrl);
    if (!this.userSession || this.userServerUrl !== baseUrl) throw new Error("当前没有有效的远程授权会话");
    try {
      const result = await this.request(baseUrl, "/v1/auth/status", { method: "GET" }, this.userSession);
      if (!isValidDate(result.expires_at) || !isValidDate(result.session_expires_at) || !isEntitlementList(result.features)) {
        throw new Error("授权服务返回的数据不完整");
      }
      return {
        expiresAt: result.expires_at,
        sessionExpiresAt: result.session_expires_at,
        features: result.features,
        safeCodeSet: Boolean(result.safe_code_set)
      };
    } catch (error) {
      this.userSession = "";
      this.userServerUrl = "";
      throw error;
    }
  }
  async logout() {
    const token = this.userSession;
    const baseUrl = this.userServerUrl;
    this.userSession = "";
    this.userServerUrl = "";
    if (token && baseUrl) await this.request(baseUrl, "/v1/auth/logout", { method: "POST" }, token);
  }
  async setSafeCode(serverUrl, code) {
    const baseUrl = this.requireUserSession(serverUrl);
    await this.request(baseUrl, "/v1/auth/safe-code", { method: "POST", body: JSON.stringify({ safe_code: code }) }, this.userSession);
  }
  async unbind(serverUrl, safeCode) {
    const baseUrl = this.requireUserSession(serverUrl);
    await this.request(baseUrl, "/v1/auth/unbind", { method: "POST", body: JSON.stringify({ safe_code: safeCode }) }, this.userSession);
    this.userSession = "";
    this.userServerUrl = "";
  }
  async adminLogin(serverUrl, username, password) {
    this.clearAdminSession();
    const baseUrl = normalizeServerUrl(serverUrl);
    const result = await this.request(baseUrl, "/v1/admin/login", {
      method: "POST",
      body: JSON.stringify({ username, password })
    });
    if (!result.token || !isValidDate(result.expires_at) || Date.parse(result.expires_at) <= Date.now()) throw new Error("管理员登录响应缺少有效会话信息");
    this.adminToken = result.token;
    this.adminServerUrl = baseUrl;
    this.adminExpiresAt = result.expires_at;
    return { expiresAt: result.expires_at };
  }
  async adminStatus(serverUrl) {
    const baseUrl = normalizeServerUrl(serverUrl);
    if (!this.adminToken || this.adminServerUrl !== baseUrl || Date.now() >= Date.parse(this.adminExpiresAt)) {
      this.clearAdminSession();
      return { loggedIn: false };
    }
    try {
      const result = await this.request(baseUrl, "/v1/admin/me", { method: "GET" }, this.adminToken);
      if (!isValidDate(result.expires_at) || Date.parse(result.expires_at) <= Date.now()) throw new Error("管理员会话已失效");
      this.adminExpiresAt = result.expires_at;
      return { loggedIn: true, expiresAt: result.expires_at };
    } catch {
      this.clearAdminSession();
      return { loggedIn: false };
    }
  }
  adminLogout() {
    this.clearAdminSession();
  }
  async listCards(serverUrl, query) {
    const baseUrl = this.requireAdminSession(serverUrl);
    const params = new URLSearchParams();
    if (query.query) params.set("q", query.query);
    if (query.status) params.set("status", query.status);
    params.set("page", String(query.page ?? 1));
    params.set("pageSize", String(query.pageSize ?? 20));
    return this.request(baseUrl, `/v1/admin/cards?${params.toString()}`, { method: "GET" }, this.adminToken);
  }
  async createCards(serverUrl, input) {
    const baseUrl = this.requireAdminSession(serverUrl);
    const result = await this.request(baseUrl, "/v1/admin/cards", {
      method: "POST",
      body: JSON.stringify(input)
    }, this.adminToken);
    return result.items;
  }
  async cardDetail(serverUrl, id) {
    const baseUrl = this.requireAdminSession(serverUrl);
    const result = await this.request(baseUrl, `/v1/admin/cards/${encodeURIComponent(id)}`, { method: "GET" }, this.adminToken);
    return result.item;
  }
  async setCardStatus(serverUrl, id, status) {
    const baseUrl = this.requireAdminSession(serverUrl);
    await this.request(baseUrl, `/v1/admin/cards/${encodeURIComponent(id)}/status`, { method: "PATCH", body: JSON.stringify({ status }) }, this.adminToken);
  }
  async resetCardBindings(serverUrl, id) {
    const baseUrl = this.requireAdminSession(serverUrl);
    const result = await this.request(baseUrl, `/v1/admin/cards/${encodeURIComponent(id)}/bindings`, { method: "DELETE" }, this.adminToken);
    return result.removed;
  }
  async extendCard(serverUrl, id, days) {
    const baseUrl = this.requireAdminSession(serverUrl);
    const result = await this.request(baseUrl, `/v1/admin/cards/${encodeURIComponent(id)}/extend`, { method: "POST", body: JSON.stringify({ days }) }, this.adminToken);
    return result.expires_at;
  }
  async auditLog(serverUrl, limit = 100) {
    const baseUrl = this.requireAdminSession(serverUrl);
    const result = await this.request(baseUrl, `/v1/admin/audit?limit=${encodeURIComponent(String(limit))}`, { method: "GET" }, this.adminToken);
    return result.items;
  }
  requireUserSession(serverUrl) {
    const baseUrl = normalizeServerUrl(serverUrl);
    if (!this.userSession || baseUrl !== this.userServerUrl) throw new Error("当前没有有效的远程授权会话");
    return baseUrl;
  }
  requireAdminSession(serverUrl) {
    const baseUrl = normalizeServerUrl(serverUrl);
    if (!this.adminToken || baseUrl !== this.adminServerUrl || Date.now() >= Date.parse(this.adminExpiresAt)) {
      this.clearAdminSession();
      throw new Error("管理员会话已失效，请重新登录");
    }
    return baseUrl;
  }
  clearAdminSession() {
    this.adminToken = "";
    this.adminServerUrl = "";
    this.adminExpiresAt = "";
  }
  async request(baseUrl, path, init, token) {
    const headers = new Headers(init.headers);
    headers.set("Accept", "application/json");
    if (init.body) headers.set("Content-Type", "application/json");
    if (token) headers.set("Authorization", `Bearer ${token}`);
    let response;
    try {
      response = await fetch(new URL(path, `${baseUrl}/`), { ...init, headers, signal: AbortSignal.timeout(1e4) });
    } catch {
      throw new Error("无法连接授权服务器，请检查地址和网络");
    }
    let payload;
    try {
      payload = await response.json();
    } catch {
      throw new Error("授权服务器返回了无效响应");
    }
    if (!response.ok || payload.ok === false) throw new Error(typeof payload.message === "string" ? payload.message : `授权服务请求失败 (${response.status})`);
    return payload;
  }
}
function normalizeServerUrl(input) {
  let url;
  try {
    url = new URL(input.trim());
  } catch {
    throw new Error("请输入有效的授权服务器地址");
  }
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash || url.pathname !== "/" && url.pathname !== "") throw new Error("授权服务器地址只能使用 http 或 https 根地址，且不能含账号、路径、参数或片段");
  const loopback = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname.toLowerCase());
  if (url.protocol !== "https:" && !loopback) throw new Error("远程授权服务器必须使用 HTTPS");
  return url.origin;
}
function isValidDate(value) {
  return typeof value === "string" && Number.isFinite(Date.parse(value));
}
function isEntitlementList(value) {
  return Array.isArray(value) && value.every((item) => item === "overlay" || item === "slot" || item === "serial");
}
function featureEntitlement(featureId) {
  switch (featureId) {
    case "green-window":
    case "virtual-camera":
    case "impact-gift":
    case "gift-screen":
      return "overlay";
    default:
      return "slot";
  }
}
const __filename$1 = fileURLToPath(import.meta.url);
const __dirname$1 = resolve(__filename$1, "..");
const APP_VERSION = "7.6.3-replica";
const SUPPORTED_PLATFORMS = ["simulator", "douyin", "kuaishou", "shipinhao", "bilibili", "tiktok", "douyu", "xiaohongshu"];
let mainWindow;
let store;
let logger;
let connectors;
let windows;
let engine;
let actionExecutor;
let settings;
let dbPath = "";
let installId = "";
let persistTimer;
let recordQueue = Promise.resolve();
const featureCooldowns = /* @__PURE__ */ new Map();
let auth = { loggedIn: false, mode: "remote", platform: "simulator", features: [] };
let authSessionExpiresAt = 0;
const licenseClient = new LicenseClient();
const obsClient = new ObsWebSocketClient();
const optionalRequire = createRequire(import.meta.url);
const defaultSettings = {
  version: APP_VERSION,
  devMode: true,
  logLevel: "info",
  retentionDays: 30,
  retentionMaxRows: 2e5,
  storeRaw: false,
  hotkey: "Ctrl + Shift + 1-9",
  audioVolume: 0.8,
  assetsRoot: "",
  overlayGreen: { visible: false, alwaysOnTop: true, opacity: 1, backgroundTransparent: false, width: 960, height: 540, laneCount: 3, background: "#00ff00" },
  overlaySlot: { visible: false, alwaysOnTop: true, opacity: 1, backgroundTransparent: true, width: 960, height: 540, laneCount: 1, background: "#000000" },
  platform: "simulator",
  roomId: "demo-room",
  authServerUrl: "http://127.0.0.1:8787",
  obsUrl: "ws://127.0.0.1:4455",
  obsPassword: "",
  autoStart: false,
  features: createDefaultFeatureSettings()
};
void bootstrap();
async function bootstrap() {
  await app.whenReady();
  app.setName("AKA直播复刻版");
  const userData = app.getPath("userData");
  const dataDir = join(userData, "data");
  const logDir = join(userData, "logs");
  const configDir = join(userData, "configs");
  await Promise.all([mkdir(dataDir, { recursive: true }), mkdir(logDir, { recursive: true }), mkdir(configDir, { recursive: true })]);
  dbPath = join(dataDir, "app.db");
  logger = new AppLogger(logDir);
  store = new SqliteStore();
  try {
    await store.init(dbPath);
  } catch (error) {
    logger.error("SQLite 初始化失败，将退出应用", error instanceof Error ? error.message : String(error));
    dialog.showErrorBox("初始化失败", "本地数据存储初始化失败，请检查应用数据目录权限。");
    app.quit();
    return;
  }
  logger.on((entry) => store.saveLog(entry));
  logger.on((entry) => push("log:append", entry));
  settings = mergeSettings(store.getSetting("settings", defaultSettings));
  settings.assetsRoot = normalizeAssetsRoot(settings.assetsRoot);
  installId = store.getSetting("authorizationInstallId", "");
  if (!installId) {
    installId = randomUUID();
    store.setSetting("authorizationInstallId", installId);
    await store.persist(dbPath);
  }
  if (!app.isPackaged) auth = { loggedIn: true, mode: "local", platform: settings.platform, features: ["overlay", "slot", "serial"] };
  windows = new WindowManager(join(__dirname$1, "../preload/index.cjs"));
  windows.updateSettings("green", settings.overlayGreen);
  windows.updateSettings("slot", settings.overlaySlot);
  windows.onMessage((message) => push("overlay:message", message));
  windows.onStatus((status) => push("overlay:status", status));
  connectors = new ConnectorManager();
  connectors.onStatus((status) => push("conn:status", status));
  actionExecutor = new ActionExecutor(windows, logger);
  engine = new RuleEngine({
    executeAction: (action, event, rule) => {
      const entitlement = actionEntitlement(action);
      if (!isAuthorized(entitlement)) return Promise.resolve({ ok: false, message: entitlement ? `当前授权未包含 ${entitlement} 权限` : "请先登录授权或进入本地开发模式" });
      return actionExecutor.execute(resolveActionPaths(action), event, rule);
    },
    log: (level, message, detail) => logger.write(level, "rule", message, detail)
  });
  connectors.onEvent((event) => {
    recordQueue = recordQueue.then(() => handleEvent(event)).catch((error) => {
      logger.error("事件处理失败", String(error));
    });
  });
  registerIpc();
  createMainWindow();
  registerDebugHotkeys();
  registerWindowHotkeys();
  const removedByRetention = store.cleanupRetention(settings.retentionDays, settings.retentionMaxRows);
  if (removedByRetention) logger.info(`[retention] removed=${removedByRetention}`);
  requestPersist();
  logger.info("应用已启动", `version=${APP_VERSION} mode=local-simulator`);
}
function createMainWindow() {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 1080,
    minHeight: 720,
    frame: false,
    show: false,
    backgroundColor: "#f4f7fb",
    webPreferences: { preload: join(__dirname$1, "../preload/index.cjs"), sandbox: true, contextIsolation: true, nodeIntegration: false }
  });
  windows.attachMainWindow(mainWindow);
  mainWindow.once("ready-to-show", () => mainWindow?.show());
  mainWindow.on("closed", () => {
    mainWindow = void 0;
  });
  mainWindow.webContents.on("preload-error", (_event, preloadPath, error) => logger.error(`[preload] ${preloadPath}`, error.message));
  mainWindow.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
  mainWindow.webContents.on("will-navigate", (event, url) => {
    if (!url.startsWith("file:") && !url.startsWith("http://127.0.0.1:")) event.preventDefault();
  });
  const rendererUrl = process.env.ELECTRON_RENDERER_URL;
  if (rendererUrl) void mainWindow.loadURL(`${rendererUrl}#/control`);
  else void mainWindow.loadFile(join(__dirname$1, "../renderer/index.html"), { hash: "/control" });
}
function registerDebugHotkeys() {
  if (!settings.devMode) return;
  for (let index = 1; index <= 9; index += 1) {
    const accelerator = `CommandOrControl+Shift+${index}`;
    const registered = globalShortcut.register(accelerator, () => {
      void debugRuleByIndex(index - 1);
    });
    if (!registered) logger.warn(`[hotkey] 无法注册 ${accelerator}`);
  }
}
function registerWindowHotkeys() {
  const registered = globalShortcut.register("CommandOrControl+F1", () => {
    void windows.toggleOpacity("slot");
  });
  if (!registered) logger.warn("[hotkey] 无法注册 CommandOrControl+F1（组件窗口透明度）");
}
async function debugRuleByIndex(index) {
  const rule = (await store.listRules())[index];
  if (!rule) {
    logger.debug(`[hotkey] 未找到第 ${index + 1} 条规则`);
    return;
  }
  const kind = rule.trigger.kinds[0] ?? "gift";
  await connectors.publish(kind === "gift" ? { kind, gift: { name: rule.trigger.giftNames?.[0] ?? rule.trigger.keywords?.[0] ?? "啤酒", count: Math.max(1, rule.trigger.minCount ?? 1) }, user: { name: "快捷键调试" } } : { kind, text: rule.trigger.keywords?.[0] ?? "调试弹幕", user: { name: "快捷键调试" } });
  logger.info(`[hotkey] sent rule=${rule.name}`);
}
function registerIpc() {
  handle("rules:list", () => store.listRules());
  handle("rules:save", async (_event, rule) => {
    const saved = await store.saveRule(normalizeRule(rule));
    requestPersist();
    return saved;
  });
  handle("rules:remove", async (_event, id) => {
    await store.removeRule(id);
    requestPersist();
  });
  handle("rules:clear", async () => {
    await store.clearRules();
    requestPersist();
  });
  handle("rules:clone", async (_event, id) => {
    const found = (await store.listRules()).find((rule) => rule.id === id);
    if (!found) throw new Error("规则不存在");
    const clone = await store.saveRule({ ...structuredClone(found), id: randomUUID(), name: `${found.name} - 副本`, enabled: false });
    requestPersist();
    return clone;
  });
  handle("danmaku:query", (_event, filter) => store.queryRecords(filter));
  handle("danmaku:count", (_event, filter) => store.countRecords(filter));
  handle("danmaku:export", async (_event, payload) => exportRecords(payload.filter, payload.format));
  handle("danmaku:clear", (_event, range) => {
    const count = store.clearRecords(range);
    requestPersist();
    logger.info(`[danmaku:store] cleared ${count} records`);
    return count;
  });
  handle("conn:connect", async (_event, platform, roomId) => {
    settings.platform = platform;
    settings.roomId = roomId;
    saveSettings();
    return connectors.connect(platform, roomId);
  });
  handle("conn:disconnect", () => connectors.disconnect());
  handle("conn:status", () => connectors.getStatus());
  handle("conn:simulate", (_event, input) => connectors.publish(input));
  handle("overlay:open", async (_event, type) => {
    requireEntitlement(type === "green" ? "overlay" : "slot");
    await windows.open(type);
    return windows.getStatus();
  });
  handle("overlay:close", async (_event, type) => {
    await windows.close(type);
    return windows.getStatus();
  });
  handle("overlay:status", () => windows.getStatus());
  handle("overlay:setMode", (_event, type, mode) => {
    requireEntitlement(type === "green" ? "overlay" : "slot");
    return windows.setMode(type, mode);
  });
  handle("overlay:toggleOpacity", async (_event, type) => {
    requireEntitlement(type === "green" ? "overlay" : "slot");
    const status = await windows.toggleOpacity(type);
    if (type === "slot") {
      settings.overlaySlot = windows.getSettings("slot");
      saveSettings();
    }
    return status;
  });
  handle("overlay:updateSettings", (_event, type, patch) => {
    requireEntitlement(type === "green" ? "overlay" : "slot");
    const updated = windows.updateSettings(type, patch);
    if (type === "green") settings.overlayGreen = updated;
    else settings.overlaySlot = updated;
    saveSettings();
    return updated;
  });
  handle("overlay:playVideo", (_event, payload) => {
    requireEntitlement("overlay");
    return windows.playVideo({ ...payload, path: resolveAssetPath(payload.path) });
  });
  handle("overlay:drop", (_event, payload) => {
    requireEntitlement("overlay");
    return windows.drop({ ...payload, image: resolveAssetPath(payload.image) });
  });
  handle("overlay:slot", (_event, payload) => {
    requireEntitlement("slot");
    return windows.slot({
      ...payload,
      images: payload.images?.map((image) => resolveAssetPath(image)),
      audioPath: payload.audioPath ? resolveAssetPath(payload.audioPath) : void 0
    });
  });
  handle("overlay:widget", (_event, payload) => {
    requireEntitlement(featureEntitlement(payload.featureId));
    const values = { ...payload.values };
    for (const key of ["imagePath", "audioPath"]) {
      if (typeof values[key] === "string" && values[key]) values[key] = resolveAssetPath(values[key]);
    }
    return windows.widget({ ...payload, values });
  });
  handle("overlay:removeWidget", (_event, featureId) => windows.removeWidget(featureId));
  handle("features:increment", (_event, featureId, key, amount) => {
    requireEntitlement("slot");
    if (featureId !== "electronic-woodfish" || key !== "currentMerit") throw new Error("不允许更新此功能状态");
    const config = settings.features[featureId];
    const current = typeof config.values[key] === "number" ? config.values[key] : 0;
    const increment = Number.isFinite(amount) ? Math.max(0, Math.floor(amount)) : 0;
    const value = Math.max(0, current + increment);
    config.values[key] = value;
    saveSettings();
    return value;
  });
  handle("audio:play", (_event, payload) => {
    requireEntitlement("slot");
    return windows.playAudio({ ...payload, path: resolveAssetPath(payload.path) });
  });
  handle("audio:stop", () => windows.stopAudio());
  handle("input:run", (_event, action) => {
    logger.info(`[input-helper] ${action.kind} action recorded in safe simulator mode`);
    return { ok: true, message: "安全模拟模式：已记录动作，未向系统注入键鼠事件" };
  });
  handle("input:findImage", (_event, image, threshold) => ({ ok: false, message: `未执行系统查图：${basename(image)} threshold=${threshold}`, confidence: 0 }));
  handle("serial:ports", async () => {
    const virtual = [{ path: "VIRTUAL-COM1", manufacturer: "Replica simulator", virtual: true }];
    try {
      const serialport = optionalRequire("serialport");
      const ports = await serialport.SerialPort.list();
      return [...virtual, ...ports.map((port) => ({ path: port.path, manufacturer: port.manufacturer, virtual: false }))];
    } catch {
      return virtual;
    }
  });
  handle("serial:pulse", (_event, action) => {
    requireEntitlement("serial");
    logger.info(`[serial] virtual pulse port=${action.port} duration=${action.pulseMs}ms`);
    return { ok: true, message: "虚拟串口模式：脉冲已记录，未写入物理设备" };
  });
  handle("serial:stopAll", () => logger.info("[serial] emergency stop: all outputs released"));
  handle("obs:connect", async (_event, url, password) => {
    requireEntitlement("overlay");
    try {
      settings.obsUrl = url;
      if (password !== void 0 && password !== "***") settings.obsPassword = password;
      saveSettings();
      const status = await obsClient.connect(url, password === void 0 || password === "***" ? settings.obsPassword : password);
      logger.info(`[obs] connected url=${url}`);
      return { ok: true, message: status.message };
    } catch (error) {
      logger.warn("[obs] connection failed", error instanceof Error ? error.message : String(error));
      return { ok: false, message: error instanceof Error ? error.message : String(error) };
    }
  });
  handle("obs:command", async (_event, command, args) => {
    requireEntitlement("overlay");
    try {
      await obsClient.command(command, args);
      if (command === "StartVirtualCam") logger.info("[obs] virtual camera started");
      if (command === "StopVirtualCam") logger.info("[obs] virtual camera stopped");
      return { ok: true, message: obsClient.status().message };
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : String(error) };
    }
  });
  handle("obs:startVirtualCamera", async () => {
    requireEntitlement("overlay");
    try {
      if (!obsClient.status().connected) await obsClient.connect(settings.obsUrl, settings.obsPassword);
      const status = await obsClient.startVirtualCamera();
      logger.info("[obs] virtual camera started");
      return { ok: true, message: status.message };
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : String(error) };
    }
  });
  handle("obs:stopVirtualCamera", async () => {
    requireEntitlement("overlay");
    try {
      const status = await obsClient.stopVirtualCamera();
      logger.info("[obs] virtual camera stopped");
      return { ok: true, message: status.message };
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : String(error) };
    }
  });
  handle("obs:status", () => obsClient.status());
  handle("features:test", async (_event, featureId) => {
    requireEntitlement(featureEntitlement(featureId));
    const config = settings.features[featureId];
    if (!config?.enabled) return { ok: false, message: "请先启用此功能" };
    if (featureId === "danmaku-assistant") {
      const keyword = featureValue(config, "keywords", "666, 欧皇").split(/[,，]/).map((item) => item.trim()).find(Boolean) ?? "666";
      featureCooldowns.delete(featureId);
      const event2 = { id: randomUUID(), source: "simulator", kind: "chat", user: { name: "测试观众" }, text: keyword, timestamp: Date.now() };
      await runFeatureRules(event2);
      return { ok: true, message: `已测试关键词回复：${keyword}` };
    }
    if (featureId === "voice-broadcast") {
      const event2 = { id: randomUUID(), kind: "chat", user: { name: "测试观众" }, text: "这是一条测试弹幕" };
      await executeFeature(featureId, config, event2, "触发");
      return { ok: true, message: "已发送测试语音播报" };
    }
    if (featureId === "virtual-camera") {
      if (!obsClient.status().connected) await obsClient.connect(settings.obsUrl, settings.obsPassword);
      const status = obsClient.status().virtualCameraActive ? await obsClient.stopVirtualCamera() : await obsClient.startVirtualCamera();
      return { ok: true, message: status.message };
    }
    const event = {
      id: randomUUID(),
      kind: "gift",
      user: { name: "功能测试" },
      gift: { name: "测试礼物", count: 1 }
    };
    await executeFeature(featureId, config, event, "触发");
    return { ok: true, message: `${featureId} 已在组件窗口/绿幕窗口执行` };
  });
  handle("features:show", async (_event, featureId) => {
    requireEntitlement(featureEntitlement(featureId));
    const config = settings.features[featureId];
    if (!config?.enabled) return { ok: false, message: "请先启用此功能" };
    const shown = await showFeaturePreview(featureId, config);
    return { ok: true, message: shown ? "组件已显示在 yapp 组件窗口" : "功能已启用，等待对应事件触发" };
  });
  handle("auth:status", async () => {
    if (auth.mode === "remote" && auth.loggedIn) {
      try {
        const session2 = await licenseClient.status(settings.authServerUrl);
        auth.expiresAt = session2.expiresAt;
        auth.features = session2.features;
        auth.safeCodeSet = session2.safeCodeSet;
        authSessionExpiresAt = Date.parse(session2.sessionExpiresAt);
      } catch {
        auth = { loggedIn: false, mode: "remote", platform: auth.platform, features: [] };
        authSessionExpiresAt = 0;
      }
    }
    return { ...auth, features: [...auth.features] };
  });
  handle("auth:login", async (_event, payload) => {
    if (!payload || !SUPPORTED_PLATFORMS.includes(payload.platform) || typeof payload.local !== "boolean") return { loggedIn: false, message: "登录参数无效" };
    authSessionExpiresAt = 0;
    auth = { loggedIn: false, mode: "remote", platform: payload.platform, features: [] };
    if (payload.local) {
      if (app.isPackaged) return { loggedIn: false, message: "正式版本不开放本地开发模式" };
      await licenseClient.logout().catch(() => void 0);
      authSessionExpiresAt = Number.POSITIVE_INFINITY;
      auth = { loggedIn: true, mode: "local", platform: payload.platform, features: ["overlay", "slot", "serial"] };
      logger.info(`[auth] local development mode platform=${payload.platform}`);
      return { loggedIn: true, message: "已进入本地开发模式" };
    }
    try {
      const session2 = await licenseClient.login(settings.authServerUrl, payload.code, payload.platform, installId);
      authSessionExpiresAt = Date.parse(session2.sessionExpiresAt);
      auth = { loggedIn: true, mode: "remote", platform: payload.platform, expiresAt: session2.expiresAt, features: session2.features, safeCodeSet: session2.safeCodeSet };
      logger.info(`[auth] remote license accepted platform=${payload.platform} features=${session2.features.join(",")}`);
      return { loggedIn: true, message: "卡密验证成功，已建立短期授权会话" };
    } catch (error) {
      logger.warn("[auth] remote license rejected", error instanceof Error ? error.message : "unknown error");
      return { loggedIn: false, message: error instanceof Error ? error.message : "卡密验证失败" };
    }
  });
  handle("auth:logout", async () => {
    await licenseClient.logout().catch(() => void 0);
    authSessionExpiresAt = 0;
    auth = { loggedIn: false, mode: "remote", platform: auth.platform, features: [] };
    logger.info("[auth] logged out");
  });
  handle("auth:setSafeCode", async (_event, code) => {
    try {
      await licenseClient.setSafeCode(settings.authServerUrl, code);
      auth.safeCodeSet = true;
      return { ok: true, message: "安全码已保存" };
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : "安全码设置失败" };
    }
  });
  handle("auth:unbind", async (_event, safeCode) => {
    try {
      await licenseClient.unbind(settings.authServerUrl, safeCode);
      auth = { loggedIn: false, mode: "remote", platform: auth.platform, features: [] };
      authSessionExpiresAt = 0;
      return { ok: true, message: "本机设备绑定已解除" };
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : "设备解绑失败" };
    }
  });
  handle("auth:adminStatus", () => licenseClient.adminStatus(settings.authServerUrl));
  handle("auth:adminLogin", async (_event, payload) => {
    const serverUrl = normalizeServerUrl(payload.serverUrl);
    const result = await licenseClient.adminLogin(serverUrl, payload.username, payload.password);
    settings.authServerUrl = serverUrl;
    saveSettings();
    logger.info("[auth-admin] administrator logged in");
    return { ok: true, message: "管理员登录成功", expiresAt: result.expiresAt };
  });
  handle("auth:adminLogout", () => licenseClient.adminLogout());
  handle("auth:listCards", (_event, query) => licenseClient.listCards(settings.authServerUrl, query));
  handle("auth:createCards", (_event, input) => licenseClient.createCards(settings.authServerUrl, input));
  handle("auth:setCardStatus", (_event, id, status) => licenseClient.setCardStatus(settings.authServerUrl, id, status));
  handle("auth:resetCardBindings", (_event, id) => licenseClient.resetCardBindings(settings.authServerUrl, id));
  handle("auth:cardDetail", (_event, id) => licenseClient.cardDetail(settings.authServerUrl, id));
  handle("auth:extendCard", (_event, id, days) => licenseClient.extendCard(settings.authServerUrl, id, days));
  handle("auth:auditLog", (_event, limit) => licenseClient.auditLog(settings.authServerUrl, limit));
  handle("config:export", () => exportConfig());
  handle("config:import", () => importConfig());
  handle("diagnostics:logs", (_event, limit) => store.listLogs(limit));
  handle("diagnostics:assets", async () => {
    try {
      return await listAssets(settings.assetsRoot);
    } catch {
      return [];
    }
  });
  handle("diagnostics:settings", () => redactSettings());
  handle("diagnostics:saveSettings", (_event, patch) => {
    settings = mergeSettings({ ...settings, ...patch });
    saveSettings();
    return redactSettings();
  });
  handle("window:minimize", () => mainWindow?.minimize());
  handle("window:maximize", () => {
    if (!mainWindow) return;
    if (mainWindow.isMaximized()) mainWindow.unmaximize();
    else mainWindow.maximize();
  });
  handle("window:close", () => mainWindow?.close());
}
async function handleEvent(event) {
  const recordId = store.insertRecord(toRecord(event));
  requestPersist();
  const initial = { ...toRecord(event), id: recordId };
  push("danmaku:append", initial);
  logger.info(`[${event.source}] ${event.kind} user=${event.user?.name ?? "匿名用户"} text=${event.text ?? event.gift?.name ?? ""}`);
  const rules = await store.listRules();
  const outcome = await engine.process(event, rules);
  await runFeatureRules(event);
  store.updateRecordResult(recordId, {
    matchedRuleId: outcome.ruleId,
    matchedRuleName: outcome.ruleName,
    actionResult: outcome.result,
    failReason: outcome.reason
  });
  const finished = { ...initial, matchedRuleId: outcome.ruleId, matchedRuleName: outcome.ruleName, actionResult: outcome.result, failReason: outcome.reason };
  push("danmaku:append", finished);
  requestPersist();
}
async function runFeatureRules(event) {
  for (const [featureId, config] of Object.entries(settings.features)) {
    if (!config?.enabled || !isAuthorized(featureEntitlement(featureId))) continue;
    try {
      if (event.kind === "gift" && event.gift?.name) {
        const giftName = event.gift.name.trim().toLocaleLowerCase();
        const enabledRules = config.giftRules?.filter((item) => item.enabled) ?? [];
        const rule = enabledRules.find((item) => item.giftName.trim() !== "*" && item.giftName.trim().toLocaleLowerCase() === giftName) ?? enabledRules.find((item) => item.giftName.trim() === "*");
        if (rule) {
          await executeFeature(featureId, config, event, rule.action, rule.values);
          logger.info(`[feature:${featureId}] gift=${event.gift.name} action=${rule.action}`);
        }
      }
      if (event.kind === "chat" && event.text?.trim()) {
        if (featureId === "danmaku-assistant") await runDanmakuAssistant(config, event);
        if (featureId === "voice-broadcast" && featureValue(config, "chatEnabled", true)) await executeFeature(featureId, config, event, "触发");
      }
    } catch (error) {
      logger.error(`[feature:${featureId}] 执行失败`, error instanceof Error ? error.message : String(error));
    }
  }
}
async function runDanmakuAssistant(config, event) {
  const keywords = splitFeatureList(featureValue(config, "keywords", "666, 欧皇"));
  const mode = featureValue(config, "keywordMode", "contains");
  const text2 = event.text?.trim() ?? "";
  const matched = keywords.some((keyword) => mode === "exact" ? text2.toLocaleLowerCase() === keyword.toLocaleLowerCase() : text2.toLocaleLowerCase().includes(keyword.toLocaleLowerCase()));
  if (!matched) return;
  const featureId = "danmaku-assistant";
  const now = Date.now();
  const cooldownMs = featureValue(config, "cooldownMs", 3e3);
  if (now - (featureCooldowns.get(featureId) ?? 0) < cooldownMs) return;
  featureCooldowns.set(featureId, now);
  await executeFeature(featureId, config, event, "触发");
}
async function showFeaturePreview(featureId, config) {
  if (windows.hasEventWidget(featureId)) {
    await windows.open("slot");
    return true;
  }
  const values = config.values;
  switch (featureId) {
    case "speed-curve":
    case "speed-iba":
      await windows.widget({ featureId, kind: "acceleration", title: featureId === "speed-curve" ? "加速度 · 曲线队列" : "加速度 · IB 批处理", values, data: { preview: true } });
      return true;
    case "frying-pan": {
      const maxValue = featureValue(config, "maxValue", 100);
      const value = typeof values.currentValue === "number" ? values.currentValue : maxValue;
      await windows.widget({ featureId, kind: "health", title: featureValue(config, "displayContent", "煮播血条"), values, data: { preview: true, value, maxValue } });
      return true;
    }
    case "impact-gift":
      await windows.widget({ featureId, kind: "notice", title: "砸礼物", values, data: { preview: true, text: "组件已启用 · 礼物效果将在绿幕窗口播放" } });
      return true;
    case "danmaku-assistant":
      await windows.widget({ featureId, kind: "reply", title: "弹幕助手", values, data: { preview: true, text: featureValue(config, "replyTemplate", "收到 {user}") } });
      return true;
    case "live-clock":
      await windows.widget({ featureId, kind: "timer", title: featureValue(config, "displayContent", "直播倒计时"), values, data: { preview: true, seconds: featureValue(config, "durationSeconds", 60) } });
      return true;
    case "electronic-woodfish": {
      const count = typeof values.currentMerit === "number" ? values.currentMerit : 0;
      await windows.widget({ featureId, kind: "woodfish", title: "电子木鱼", values, data: { preview: true, count } });
      return true;
    }
    case "lottery":
      await windows.widget({ featureId, kind: "wheel", title: "大转盘", values, data: { preview: true } });
      return true;
    case "counter": {
      const value = typeof values.currentValue === "number" ? values.currentValue : featureValue(config, "initialValue", 0);
      await windows.widget({ featureId, kind: "counter", title: featureValue(config, "displayContent", "礼物计数"), values, data: { preview: true, value } });
      return true;
    }
    case "gift-screen":
      await windows.widget({ featureId, kind: "notice", title: "礼物飘屏", values, data: { preview: true, text: "组件已启用 · 礼物素材将在绿幕窗口飘屏" } });
      return true;
    case "gift-pool": {
      const selected = splitFeatureList(featureValue(config, "pool", "啤酒, 小心心, 平底锅"))[0] ?? "等待礼物";
      const isImage = /\.(png|jpe?g|gif|webp|svg)$/i.test(selected);
      await windows.widget({ featureId, kind: "sticker", title: "礼物咖", values, data: { preview: true, sticker: isImage ? basename(selected) : selected, imagePath: isImage ? pathToFileURL(resolveAssetPath(selected)).href : "" } });
      return true;
    }
    case "screen-lock":
      await windows.widget({ featureId, kind: "lock", title: "屏幕锁键", values, data: { preview: true, locked: false } });
      return true;
    case "mosquito-slap":
      await windows.widget({ featureId, kind: "mosquito", title: "拍蚊子", values: { ...values, imagePath: pathToFileURL(resolveAssetPath(featureValue(config, "imagePath", "idle.png"))).href }, data: { preview: true, durationMs: featureValue(config, "durationMs", 2e4), score: 0 } });
      return true;
    default:
      return false;
  }
}
async function executeFeature(featureId, config, event, ruleAction, ruleValues) {
  requireEntitlement(featureEntitlement(featureId));
  const activeConfig = ruleValues ? { ...config, values: { ...config.values, ...ruleValues } } : config;
  const persistValue = (key, value) => {
    config.values[key] = value;
    activeConfig.values[key] = value;
  };
  const giftCount = Math.min(100, Math.max(1, event.gift?.count ?? 1));
  switch (featureId) {
    case "green-window": {
      await windows.updateSettings("green", { background: featureValue(activeConfig, "backgroundColor", "#00ff00"), laneCount: featureValue(activeConfig, "laneCount", 3), alwaysOnTop: featureValue(activeConfig, "alwaysOnTop", true) });
      await windows.open("green");
      const videoPath = featureValue(activeConfig, "videoPath", "").trim();
      if (videoPath) await windows.playVideo({ path: resolveAssetPath(videoPath), durationMs: featureValue(activeConfig, "videoDurationMs", 8e3), loop: featureValue(activeConfig, "videoLoop", false) });
      break;
    }
    case "component-window":
      await windows.updateSettings("slot", { width: featureValue(activeConfig, "width", 960), height: featureValue(activeConfig, "height", 540), alwaysOnTop: featureValue(activeConfig, "alwaysOnTop", true) });
      await windows.open("slot");
      break;
    case "virtual-camera":
      if (!obsClient.status().connected) await obsClient.connect(settings.obsUrl, settings.obsPassword);
      await obsClient.startVirtualCamera();
      break;
    case "speed-curve":
    case "speed-iba":
      await windows.widget({
        featureId,
        kind: "acceleration",
        title: featureId === "speed-curve" ? "加速度 · 曲线队列" : "加速度 · IB 批处理",
        values: activeConfig.values,
        data: { targetCount: Math.min(9999, featureValue(activeConfig, "targetCount", 10) * giftCount), giftCount }
      });
      break;
    case "impact-gift": {
      const count = Math.min(100, featureValue(activeConfig, "count", 8) * giftCount);
      await windows.drop({ image: resolveAssetPath(featureValue(activeConfig, "imagePath", "images/平底锅.png")), count, gravity: featureValue(activeConfig, "gravity", 1800), bounce: featureValue(activeConfig, "bounce", 0.45), durationMs: 6e3 });
      await windows.widget({ featureId, kind: "notice", title: "砸礼物", values: activeConfig.values, data: { text: `${event.gift?.name ?? "礼物"} × ${count} · 已发送到绿幕窗口` } });
      break;
    }
    case "frying-pan": {
      const maxValue = featureValue(activeConfig, "maxValue", 100);
      const current = typeof config.values.currentValue === "number" ? config.values.currentValue : maxValue;
      const change = featureValue(activeConfig, "damagePerGift", 10) * giftCount * (ruleAction === "减少" ? -1 : 1);
      const value = Math.min(maxValue, Math.max(0, current + change));
      persistValue("currentValue", value);
      saveSettings();
      await windows.widget({ featureId, kind: "health", title: featureValue(activeConfig, "displayContent", "煮播血条"), values: activeConfig.values, data: { value, maxValue } });
      break;
    }
    case "voice-broadcast": {
      const isChat = event.kind === "chat";
      const template = featureValue(activeConfig, isChat ? "chatTemplate" : "template", isChat ? "{user} 说 {text}" : "{user} 送出 {gift}，数量 {count}");
      const text2 = formatFeatureTemplate(template, event);
      await windows.widget({ featureId, kind: "speech", title: "语音播报", values: activeConfig.values, data: { text: text2, volume: featureValue(activeConfig, "volume", 0.8), interrupt: featureValue(activeConfig, "interrupt", false) } });
      break;
    }
    case "danmaku-assistant": {
      const reply = formatFeatureTemplate(featureValue(activeConfig, "replyTemplate", "收到 {user}"), event);
      await windows.widget({ featureId, kind: "reply", title: "弹幕助手 · 本地回复预览", values: activeConfig.values, data: { text: reply, user: event.user?.name ?? "观众" } });
      logger.info(`[feature:danmaku-assistant] local reply preview: ${reply}`);
      break;
    }
    case "live-clock": {
      const action = event.kind === "gift" && ruleAction === "增加" ? "add" : event.kind === "gift" && ruleAction === "减少" ? "subtract" : "start";
      await windows.widget({
        featureId,
        kind: "timer",
        title: featureValue(activeConfig, "displayContent", "直播倒计时"),
        values: activeConfig.values,
        data: { action, seconds: featureValue(activeConfig, "durationSeconds", 60), deltaSeconds: featureValue(activeConfig, "secondsPerGift", 10) * giftCount }
      });
      break;
    }
    case "electronic-woodfish": {
      const previous = typeof config.values.currentMerit === "number" ? config.values.currentMerit : 0;
      const perGift = featureValue(activeConfig, "meritPerGift", featureValue(activeConfig, "meritPerClick", 1));
      const count = event.kind === "gift" ? previous + perGift * giftCount : previous;
      if (event.kind === "gift") {
        persistValue("currentMerit", count);
        saveSettings();
      }
      await windows.widget({ featureId, kind: "woodfish", title: "电子木鱼", values: activeConfig.values, data: { count } });
      break;
    }
    case "slot-machine":
      await runFeatureSlot(activeConfig);
      break;
    case "gift-screen":
      await windows.drop({ image: resolveAssetPath(featureValue(activeConfig, "imagePath", "images/平底锅.png")), count: giftCount, durationMs: featureValue(activeConfig, "durationMs", 3500), gravity: 500, bounce: 0.2, maxVisible: featureValue(activeConfig, "maxVisible", 20) });
      await windows.widget({ featureId, kind: "notice", title: "礼物飘屏", values: activeConfig.values, data: { text: `${event.gift?.name ?? "礼物"} × ${giftCount} · 已发送到绿幕窗口` } });
      break;
    case "lottery":
      await windows.widget({ featureId, kind: "wheel", title: "大转盘", values: activeConfig.values });
      break;
    case "counter": {
      const current = typeof config.values.currentValue === "number" ? config.values.currentValue : featureValue(activeConfig, "initialValue", 0);
      const direction = ruleAction === "减少" ? -1 : 1;
      const value = Math.max(0, current + featureValue(activeConfig, "step", 1) * giftCount * direction);
      persistValue("currentValue", value);
      saveSettings();
      await windows.widget({ featureId, kind: "counter", title: featureValue(activeConfig, "displayContent", "礼物计数"), values: activeConfig.values, data: { value } });
      break;
    }
    case "gift-pool": {
      const pool = splitFeatureList(featureValue(activeConfig, "pool", "啤酒, 小心心, 平底锅"));
      if (!pool.length) break;
      let index = Math.floor(Math.random() * pool.length);
      if (!featureValue(activeConfig, "randomize", true)) {
        index = Number(config.values.nextStickerIndex ?? 0) % pool.length;
        persistValue("nextStickerIndex", (index + 1) % pool.length);
        saveSettings();
      }
      const selected = pool[index];
      const isImage = /\.(png|jpe?g|gif|webp|svg)$/i.test(selected);
      await windows.widget({
        featureId,
        kind: "sticker",
        title: "礼物咖",
        values: activeConfig.values,
        data: { sticker: isImage ? basename(selected) : selected, imagePath: isImage ? pathToFileURL(resolveAssetPath(selected)).href : "", durationMs: featureValue(activeConfig, "durationMs", 3500) }
      });
      break;
    }
    case "screen-lock":
      await windows.widget({ featureId, kind: "lock", title: "屏幕锁键", values: activeConfig.values, data: { locked: true } });
      break;
    case "mosquito-slap":
      await windows.widget({
        featureId,
        kind: "mosquito",
        title: "拍蚊子",
        values: { ...activeConfig.values, imagePath: pathToFileURL(resolveAssetPath(featureValue(activeConfig, "imagePath", "idle.png"))).href },
        data: { durationMs: featureValue(activeConfig, "durationMs", 2e4), score: 0 }
      });
      break;
  }
}
async function runFeatureSlot(config) {
  const pool = splitFeatureList(featureValue(config, "pool", "一等奖, 二等奖, 谢谢参与"));
  const weights = parseFeatureWeights(featureValue(config, "weights", "1, 10, 89"), pool.length);
  const audioPath = featureValue(config, "playMusic", true) ? resolveAssetPath("fruit.mp3") : void 0;
  await windows.slot({
    theme: featureValue(config, "theme", "default"),
    pool: pool.length ? pool : ["谢谢参与"],
    weights,
    images: Array.from({ length: 14 }, (_, index) => resolveAssetPath(`水果机/${index + 1}.png`)),
    durationMs: featureValue(config, "durationMs", 2200),
    audioPath
  });
}
function splitFeatureList(value) {
  return value.split(/[,，]/).map((item) => item.trim()).filter(Boolean);
}
function parseFeatureWeights(value, length) {
  const parsed = splitFeatureList(value).map((item) => Number(item)).map((item) => Number.isFinite(item) && item >= 0 ? item : 0);
  const values = Array.from({ length }, (_, index) => parsed[index] ?? 1);
  return values.some((item) => item > 0) ? values : values.map(() => 1);
}
function formatFeatureTemplate(template, event) {
  return template.replaceAll("{user}", event.user?.name ?? "观众").replaceAll("{text}", event.text ?? "").replaceAll("{gift}", event.gift?.name ?? "弹幕").replaceAll("{count}", String(event.gift?.count ?? event.count ?? 1));
}
function featureValue(config, key, fallback) {
  const value = config.values[key];
  return typeof value === typeof fallback ? value : fallback;
}
function toRecord(event) {
  return {
    source: event.source,
    roomId: event.roomId,
    kind: event.kind,
    userId: event.user?.id,
    userName: event.user?.name,
    text: event.text,
    giftName: event.gift?.name,
    giftCount: event.gift?.count,
    giftValue: event.gift?.value,
    repeatCount: event.kind === "like" ? event.count : void 0,
    ts: event.timestamp,
    createdAt: Date.now(),
    actionResult: "none",
    rawJson: settings.storeRaw ? JSON.stringify(event.raw ?? {}) : void 0
  };
}
function normalizeRule(rule) {
  return {
    ...rule,
    id: rule.id || randomUUID(),
    name: rule.name.trim() || "未命名玩法",
    priority: Number.isFinite(rule.priority) ? rule.priority : 0,
    trigger: { ...rule.trigger, kinds: rule.trigger.kinds?.length ? rule.trigger.kinds : ["gift"] },
    concurrency: rule.concurrency ?? "queue",
    actions: rule.actions ?? []
  };
}
function resolveActionPaths(action) {
  if (action.kind === "video" || action.kind === "audio") return { ...action, path: resolveAssetPath(action.path) };
  if (action.kind === "drop") return { ...action, image: resolveAssetPath(action.image) };
  return action;
}
function resolveAssetPath(input) {
  const root = resolve(settings.assetsRoot);
  const candidate = resolve(isAbsolute(input) ? input : join(root, input));
  if (candidate !== root && !candidate.startsWith(`${root}${sep}`)) throw new Error("素材路径必须位于素材目录内");
  return candidate;
}
function defaultAssetsRoot() {
  const candidates = app.isPackaged ? [join(process.resourcesPath, "resources")] : [join(app.getAppPath(), "resources"), join(app.getAppPath(), "..", "resources"), join(__dirname$1, "..", "..", "resources")];
  return candidates.find((candidate) => existsSync(candidate)) ?? candidates[0];
}
function normalizeAssetsRoot(input) {
  const candidate = input ? isAbsolute(input) ? input : resolve(app.getAppPath(), input) : defaultAssetsRoot();
  return existsSync(candidate) ? candidate : defaultAssetsRoot();
}
async function exportRecords(filter, format) {
  const records = store.queryRecords({ ...filter, limit: 500 });
  const content = format === "json" ? JSON.stringify(records, null, 2) : format === "csv" ? toCsv(records) : records.map(formatRecordText).join("\n");
  const result = await dialog.showSaveDialog(mainWindow, { defaultPath: join(app.getPath("documents"), `danmaku-${Date.now()}.${format}`), filters: [{ name: format.toUpperCase(), extensions: [format] }] });
  if (result.canceled || !result.filePath) return { content };
  await writeFile(result.filePath, content, "utf8");
  logger.info(`[danmaku:export] ${format} rows=${records.length}`);
  return { path: result.filePath };
}
async function exportConfig() {
  const config = await buildExportConfig();
  const result = await dialog.showSaveDialog(mainWindow, { defaultPath: join(app.getPath("documents"), "abi-replica-config.json"), filters: [{ name: "JSON", extensions: ["json"] }] });
  if (result.canceled || !result.filePath) return { ok: false, message: "已取消导出" };
  if (existsSync(result.filePath)) await writeFile(`${result.filePath}.bak`, await readFile(result.filePath));
  await writeFile(result.filePath, JSON.stringify(config, null, 2), "utf8");
  logger.info(`[config] exported ${basename(result.filePath)}`);
  return { ok: true, path: result.filePath, message: "配置导出成功" };
}
async function importConfig() {
  const result = await dialog.showOpenDialog(mainWindow, { properties: ["openFile"], filters: [{ name: "JSON", extensions: ["json"] }] });
  if (result.canceled || !result.filePaths[0]) return { ok: false, message: "已取消导入" };
  let parsed;
  try {
    parsed = JSON.parse(await readFile(result.filePaths[0], "utf8"));
  } catch {
    return { ok: false, message: "配置不是有效 JSON" };
  }
  if (parsed.schema_version !== 1 || !Array.isArray(parsed.rules)) return { ok: false, message: "不支持的配置版本或缺少 rules" };
  for (const rule of parsed.rules) await store.saveRule(normalizeRule(rule));
  if (parsed.overlay?.green) {
    settings.overlayGreen = mergeSettings({ overlayGreen: parsed.overlay.green }).overlayGreen;
    windows.updateSettings("green", settings.overlayGreen);
  }
  if (parsed.overlay?.slot) {
    settings.overlaySlot = mergeSettings({ overlaySlot: parsed.overlay.slot }).overlaySlot;
    windows.updateSettings("slot", settings.overlaySlot);
  }
  if (parsed.connectors?.platform) {
    settings.platform = parsed.connectors.platform;
    settings.roomId = parsed.connectors.roomId ?? settings.roomId;
  }
  if (parsed.danmaku) settings = mergeSettings({ ...settings, retentionDays: parsed.danmaku.retention_days ?? settings.retentionDays, retentionMaxRows: parsed.danmaku.retention_max_rows ?? settings.retentionMaxRows, storeRaw: parsed.danmaku.store_raw ?? settings.storeRaw });
  if (parsed.features) settings.features = createDefaultFeatureSettings(parsed.features);
  saveSettings();
  requestPersist();
  logger.info(`[config] imported ${parsed.rules.length} rules`);
  return { ok: true, message: `已导入 ${parsed.rules.length} 条规则`, rules: await store.listRules() };
}
async function buildExportConfig() {
  return {
    schema_version: 1,
    app_version: APP_VERSION,
    rules: await store.listRules(),
    assets: await listAssets(settings.assetsRoot),
    overlay: { green: settings.overlayGreen, slot: settings.overlaySlot },
    slot: { theme: "default" },
    connectors: { platform: settings.platform, roomId: settings.roomId },
    danmaku: { retention_days: settings.retentionDays, retention_max_rows: settings.retentionMaxRows, store_raw: settings.storeRaw },
    features: settings.features
  };
}
async function listAssets(root) {
  root = normalizeAssetsRoot(root);
  const output = [];
  async function walk(directory) {
    if (!existsSync(directory)) return;
    for (const item of await readdir(directory, { withFileTypes: true })) {
      const full = join(directory, item.name);
      if (item.isDirectory()) await walk(full);
      else if ([".png", ".jpg", ".jpeg", ".bmp", ".gif", ".svg", ".mp3", ".wav", ".mp4"].includes(extname(item.name).toLowerCase())) output.push(relative(root, full));
    }
  }
  await walk(root);
  return output.sort();
}
function mergeSettings(input) {
  return {
    ...defaultSettings,
    ...input,
    overlayGreen: { ...defaultSettings.overlayGreen, ...input.overlayGreen ?? {} },
    overlaySlot: { ...defaultSettings.overlaySlot, ...input.overlaySlot ?? {} },
    features: createDefaultFeatureSettings(input.features)
  };
}
function isAuthorized(entitlement) {
  if (!auth.loggedIn) return false;
  if (auth.mode === "local") return !app.isPackaged;
  if (Date.now() >= authSessionExpiresAt) return false;
  return entitlement === void 0 || auth.features.includes(entitlement);
}
function requireEntitlement(entitlement) {
  if (!isAuthorized(entitlement)) throw new Error(`当前卡密未包含 ${entitlement} 权限，或授权会话已过期`);
}
function actionEntitlement(action) {
  switch (action.kind) {
    case "video":
    case "drop":
    case "obs":
      return "overlay";
    case "slot":
    case "audio":
      return "slot";
    case "serial":
      return "serial";
    default:
      return void 0;
  }
}
function saveSettings() {
  store.setSetting("settings", settings);
  requestPersist();
}
function redactSettings() {
  return { ...settings, obsPassword: settings.obsPassword ? "***" : "" };
}
function requestPersist() {
  if (persistTimer) clearTimeout(persistTimer);
  persistTimer = setTimeout(() => {
    persistTimer = void 0;
    void store.persist(dbPath);
  }, 250);
}
function push(channel, payload) {
  if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send(channel, payload);
}
function handle(channel, listener) {
  ipcMain.handle(channel, async (event, ...args) => {
    const needsAuthorization = [
      "conn:connect",
      "conn:simulate",
      "features:",
      "overlay:open",
      "overlay:setMode",
      "overlay:toggleOpacity",
      "overlay:updateSettings",
      "overlay:playVideo",
      "overlay:drop",
      "overlay:slot",
      "overlay:widget",
      "audio:play",
      "input:",
      "serial:pulse",
      "obs:connect",
      "obs:command",
      "obs:startVirtualCamera",
      "obs:stopVirtualCamera"
    ].some((prefix) => prefix.endsWith(":") ? channel.startsWith(prefix) : channel === prefix);
    if (needsAuthorization && !isAuthorized()) throw new Error("请使用有效卡密登录，或在开发版中进入本地模式");
    return listener(event, ...args);
  });
}
function toCsv(records) {
  const header = ["时间", "平台", "类型", "昵称", "内容", "命中规则", "结果"];
  const rows = records.map((record) => [new Date(record.ts).toLocaleString("zh-CN"), record.source, record.kind, record.userName ?? "", record.text ?? record.giftName ?? "", record.matchedRuleName ?? "", record.actionResult]);
  return [header, ...rows].map((row) => row.map((value) => `"${String(value).replaceAll('"', '""')}"`).join(",")).join("\n");
}
function formatRecordText(record) {
  return `[${new Date(record.ts).toLocaleTimeString("zh-CN")}] [${record.source}] ${record.userName ?? "匿名用户"} ${record.text ?? record.giftName ?? record.kind} ${record.actionResult}`;
}
app.on("before-quit", () => {
  globalShortcut.unregisterAll();
  obsClient.disconnect();
  windows?.markQuitting();
  windows?.destroyAll();
  void store?.persist(dbPath);
});
app.on("window-all-closed", () => {
  if (process.platform !== "darwin") app.quit();
});
app.on("activate", () => {
  if (!mainWindow && store) createMainWindow();
});
app.whenReady().then(() => session.defaultSession.setPermissionRequestHandler((_webContents, _permission, callback) => callback(false))).catch(() => void 0);
