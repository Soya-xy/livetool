package main

import "time"

type Platform string

const (
	PlatformDouyin      Platform = "douyin"
	PlatformKuaishou    Platform = "kuaishou"
	PlatformShipinhao   Platform = "shipinhao"
	PlatformBilibili    Platform = "bilibili"
	PlatformTikTok      Platform = "tiktok"
	PlatformDouyu       Platform = "douyu"
	PlatformXiaohongshu Platform = "xiaohongshu"
	PlatformSimulator   Platform = "simulator"
)

var supportedPlatforms = []Platform{PlatformDouyin, PlatformKuaishou, PlatformShipinhao, PlatformBilibili, PlatformTikTok, PlatformDouyu, PlatformXiaohongshu, PlatformSimulator}

type EventKind string

const (
	KindGift   EventKind = "gift"
	KindChat   EventKind = "chat"
	KindLike   EventKind = "like"
	KindFollow EventKind = "follow"
	KindEnter  EventKind = "enter"
	KindSystem EventKind = "system"
)

type LiveEvent struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	RoomID    string         `json:"roomId,omitempty"`
	Kind      EventKind      `json:"kind"`
	User      *LiveUser      `json:"user,omitempty"`
	Gift      *LiveGift      `json:"gift,omitempty"`
	Text      string         `json:"text,omitempty"`
	Count     int            `json:"count,omitempty"`
	Timestamp int64          `json:"timestamp"`
	Raw       map[string]any `json:"raw,omitempty"`
}

type LiveUser struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type LiveGift struct {
	ID    string  `json:"id,omitempty"`
	Name  string  `json:"name"`
	Icon  string  `json:"icon,omitempty"`
	Count int     `json:"count"`
	Value float64 `json:"value,omitempty"`
}

type WindowTarget struct {
	Title       string `json:"title,omitempty"`
	ClassName   string `json:"className,omitempty"`
	ProcessName string `json:"processName,omitempty"`
}

type ChromaConfig struct {
	Enabled    bool    `json:"enabled"`
	Color      string  `json:"color"`
	Similarity float64 `json:"similarity"`
	Smoothness float64 `json:"smoothness"`
}

type OverlayVideoPayload struct {
	Path       string        `json:"path"`
	Lane       int           `json:"lane,omitempty"`
	DurationMS int           `json:"durationMs,omitempty"`
	Loop       bool          `json:"loop,omitempty"`
	Chroma     *ChromaConfig `json:"chroma,omitempty"`
}

type ImageSearchResult struct {
	OK         bool    `json:"ok"`
	Message    string  `json:"message"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Action is intentionally a compact JSON-compatible union. The renderer
// persists the original action fields; only the selected action kind is used.
type Action struct {
	ID         string            `json:"id,omitempty"`
	Kind       string            `json:"kind"`
	DelayMS    int               `json:"delayMs,omitempty"`
	Repeat     int               `json:"repeat,omitempty"`
	DurationMS int               `json:"durationMs,omitempty"`
	Path       string            `json:"path,omitempty"`
	Image      string            `json:"image,omitempty"`
	Lane       int               `json:"lane,omitempty"`
	Loop       bool              `json:"loop,omitempty"`
	Volume     float64           `json:"volume,omitempty"`
	Interrupt  bool              `json:"interrupt,omitempty"`
	Count      int               `json:"count,omitempty"`
	Gravity    float64           `json:"gravity,omitempty"`
	Bounce     float64           `json:"bounce,omitempty"`
	MaxVisible int               `json:"maxVisible,omitempty"`
	Theme      string            `json:"theme,omitempty"`
	Pool       []string          `json:"pool,omitempty"`
	Weights    []float64         `json:"weights,omitempty"`
	Port       string            `json:"port,omitempty"`
	Baud       int               `json:"baud,omitempty"`
	OnBytes    []int             `json:"onBytes,omitempty"`
	OffBytes   []int             `json:"offBytes,omitempty"`
	PulseMS    int               `json:"pulseMs,omitempty"`
	Command    string            `json:"command,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Steps      []map[string]any  `json:"steps,omitempty"`
	Target     *WindowTarget     `json:"target,omitempty"`
	Chroma     *ChromaConfig     `json:"chroma,omitempty"`

	// ── 砸图片（showimage）：礼物砸向屏幕的大小与落点 ──
	Size int `json:"size,omitempty"`
	X    int `json:"x,omitempty"`
	Y    int `json:"y,omitempty"`

	// ── 手机玩法（app）：app_action 默认 video ──
	AppAction string `json:"appAction,omitempty"`

	// ── 加速度（gospeed）：到达数量与倍速（1.0–2.0） ──
	TargetCount int     `json:"targetCount,omitempty"`
	Speed       float64 `json:"speed,omitempty"`

	// ── OBS场景滤镜（obs_filter） ──
	FilterName string `json:"filterName,omitempty"`
	Visible    bool   `json:"visible,omitempty"`
	AutoHide   bool   `json:"autoHide,omitempty"`

	// ── 内置事件（rule）──
	// RuleEvent 取值见 builtin_events.go 的常量；RuleEventTimeoutMS 只对
	// 锁定类事件生效（原版 rule_event_timeout，默认 1000）。
	RuleEvent          string `json:"ruleEvent,omitempty"`
	RuleEventTimeoutMS int    `json:"ruleEventTimeoutMs,omitempty"`
	// KillProcessName 仅 kill_process_name 使用，RunExePath 仅 run_exe 使用。
	KillProcessName string `json:"killProcessName,omitempty"`
	RunExePath      string `json:"runExePath,omitempty"`

	Delta int `json:"delta,omitempty"`

	// ── 屏幕锁链（tielian）：Effect 取值 增加 / 减少。 ──
	Effect   string `json:"effect,omitempty"`
	CountMin int    `json:"countMin,omitempty"`
	CountMax int    `json:"countMax,omitempty"`
	TrashBin string `json:"bin,omitempty"`
}

type RuleTrigger struct {
	Kinds       []EventKind `json:"kinds"`
	GiftNames   []string    `json:"giftNames,omitempty"`
	Keywords    []string    `json:"keywords,omitempty"`
	KeywordMode string      `json:"keywordMode,omitempty"`
	MinCount    int         `json:"minCount,omitempty"`
	Users       []string    `json:"users,omitempty"`
	Source      []string    `json:"source,omitempty"`
}

// RuleTriggerLimits mirrors the original app's 「触发限制」: within the given
// number of seconds the same user may only trigger this rule once. Zero (or a
// missing limits object) means no limit.
type RuleTriggerLimits struct {
	GiftSecond  int `json:"giftSecond,omitempty"`
	TextSecond  int `json:"textSecond,omitempty"`
	LikeSecond  int `json:"likeSecond,omitempty"`
	EnterSecond int `json:"enterSecond,omitempty"`
}

type Rule struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Enabled     bool        `json:"enabled"`
	Priority    int         `json:"priority"`
	Trigger     RuleTrigger `json:"trigger"`
	CooldownMS  int         `json:"cooldownMs,omitempty"`
	Probability *float64    `json:"probability,omitempty"`
	Concurrency string      `json:"concurrency"`
	Actions     []Action    `json:"actions"`
	CreatedAt   int64       `json:"createdAt,omitempty"`
	UpdatedAt   int64       `json:"updatedAt,omitempty"`
	// Pinned keeps the rule at the top of the control centre list.
	Pinned bool `json:"pinned,omitempty"`
	// Hotkey is the optional global shortcut that triggers this rule directly.
	Hotkey string `json:"hotkey,omitempty"`
	// RepeatCount runs the whole action list this many times (default 1).
	RepeatCount int `json:"repeatCount,omitempty"`
	// Nowait skips the per-rule queue and executes immediately instead.
	Nowait bool `json:"nowait,omitempty"`
	// WaitMS 排队执行时的等待时长（毫秒）：动作开始前先等这么久，0 表示不等待。
	WaitMS        int                `json:"waitMs,omitempty"`
	TriggerLimits *RuleTriggerLimits `json:"triggerLimits,omitempty"`
}

// RulesRuntimeState is the control centre footer state: the global rules switch
// plus the two fixed global hotkeys that drive it.
type RulesRuntimeState struct {
	Enabled bool `json:"enabled"`
	Paused  bool `json:"paused"`
	// DebugKeys reports whether the Ctrl + Shift + 1-9 debug shortcuts are active.
	DebugKeys bool   `json:"debugKeys"`
	OpenKey   string `json:"openKey"`
	CloseKey  string `json:"closeKey"`
}

// inputOptions carries the 通用设置 values that affect native input execution.
type inputOptions struct {
	MoveVerticalStep   int
	MoveHorizontalStep int
}

type DanmakuRecord struct {
	ID              int64     `json:"id"`
	Source          string    `json:"source"`
	RoomID          string    `json:"roomId,omitempty"`
	Kind            EventKind `json:"kind"`
	UserID          string    `json:"userId,omitempty"`
	UserName        string    `json:"userName,omitempty"`
	Text            string    `json:"text,omitempty"`
	GiftName        string    `json:"giftName,omitempty"`
	GiftCount       int       `json:"giftCount,omitempty"`
	GiftValue       float64   `json:"giftValue,omitempty"`
	RepeatCount     int       `json:"repeatCount,omitempty"`
	TS              int64     `json:"ts"`
	CreatedAt       int64     `json:"createdAt"`
	MatchedRuleID   string    `json:"matchedRuleId,omitempty"`
	MatchedRuleName string    `json:"matchedRuleName,omitempty"`
	ActionResult    string    `json:"actionResult"`
	FailReason      string    `json:"failReason,omitempty"`
	RawJSON         string    `json:"rawJson,omitempty"`
}

type DanmakuFilter struct {
	From         int64  `json:"from,omitempty"`
	To           int64  `json:"to,omitempty"`
	Source       string `json:"source,omitempty"`
	Kind         string `json:"kind,omitempty"`
	UserName     string `json:"userName,omitempty"`
	Keyword      string `json:"keyword,omitempty"`
	ActionResult string `json:"actionResult,omitempty"`
	Matched      string `json:"matched,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Offset       int    `json:"offset,omitempty"`
}

type ConnectorStatus struct {
	Platform    string `json:"platform"`
	RoomID      string `json:"roomId,omitempty"`
	State       string `json:"state"`
	Mode        string `json:"mode"`
	Reconnects  int    `json:"reconnects"`
	Dropped     int    `json:"dropped"`
	LastError   string `json:"lastError,omitempty"`
	LastEventAt int64  `json:"lastEventAt,omitempty"`
}

// WidgetLayout 是组件窗里某个组件的位置与大小（拖动 / 拉伸后保存）。
type WidgetLayout struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type SerialPort struct {
	Path         string `json:"path"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Virtual      bool   `json:"virtual"`
}

type OverlaySettings struct {
	Visible               bool    `json:"visible"`
	AlwaysOnTop           bool    `json:"alwaysOnTop"`
	Opacity               float64 `json:"opacity"`
	BackgroundTransparent bool    `json:"backgroundTransparent"`
	Width                 int     `json:"width"`
	Height                int     `json:"height"`
	LaneCount             int     `json:"laneCount"`
	Background            string  `json:"background"`
	// ShowPerf 打开后组件窗角落显示帧率与长帧统计，用于定位拖动 / 拉伸卡顿。
	ShowPerf bool `json:"showPerf,omitempty"`
}

type OverlayWindowStatus struct {
	Type                  string  `json:"type"`
	Role                  string  `json:"role"`
	Title                 string  `json:"title"`
	Visible               bool    `json:"visible"`
	Width                 int     `json:"width"`
	Height                int     `json:"height"`
	Mode                  string  `json:"mode"`
	Fullscreen            bool    `json:"fullscreen"`
	Opacity               float64 `json:"opacity"`
	BackgroundTransparent bool    `json:"backgroundTransparent"`
	ShowPerf              bool    `json:"showPerf"`
}

type FeatureID string

const (
	FeatureGreenWindow      FeatureID = "green-window"
	FeatureComponentWindow  FeatureID = "component-window"
	FeatureVirtualCamera    FeatureID = "virtual-camera"
	FeatureSpeedCurve       FeatureID = "speed-curve"
	FeatureSpeedIba         FeatureID = "speed-iba"
	FeatureImpactGift       FeatureID = "impact-gift"
	FeatureFryingPan        FeatureID = "frying-pan"
	FeatureVoiceBroadcast   FeatureID = "voice-broadcast"
	FeatureDanmakuAssistant FeatureID = "danmaku-assistant"
	FeatureLiveClock        FeatureID = "live-clock"
	FeatureGiftScreen       FeatureID = "gift-screen"
	FeatureGiftPool         FeatureID = "gift-pool"
	FeatureScreenLock       FeatureID = "screen-lock"
	// 新版新增：倒计时 / 垃圾掉落。
	FeatureCountdown FeatureID = "countdown"
	FeatureTrashDrop FeatureID = "trash-drop"
)

type FeatureGiftRule struct {
	ID       string         `json:"id"`
	GiftName string         `json:"giftName"`
	Action   string         `json:"action"`
	Enabled  bool           `json:"enabled"`
	Values   map[string]any `json:"values,omitempty"`
}

type FeatureConfig struct {
	Enabled   bool              `json:"enabled"`
	Values    map[string]any    `json:"values"`
	GiftRules []FeatureGiftRule `json:"giftRules"`
}

type FeatureSettings map[FeatureID]FeatureConfig

type OverlayWidgetPayload struct {
	FeatureID FeatureID      `json:"featureId"`
	Kind      string         `json:"kind"`
	Title     string         `json:"title"`
	Values    map[string]any `json:"values"`
	Data      map[string]any `json:"data,omitempty"`
}

type AppSettings struct {
	Version          string          `json:"version"`
	DevMode          bool            `json:"devMode"`
	LogLevel         string          `json:"logLevel"`
	RetentionDays    int             `json:"retentionDays"`
	RetentionMaxRows int             `json:"retentionMaxRows"`
	StoreRaw         bool            `json:"storeRaw"`
	Hotkey           string          `json:"hotkey"`
	AudioVolume      float64         `json:"audioVolume"`
	AssetsRoot       string          `json:"assetsRoot"`
	OverlayGreen     OverlaySettings `json:"overlayGreen"`
	OverlaySlot      OverlaySettings `json:"overlaySlot"`
	Platform         Platform        `json:"platform"`
	RoomID           string          `json:"roomId"`
	AuthServerURL    string          `json:"authServerUrl"`
	OBSURL           string          `json:"obsUrl"`
	OBSPassword      string          `json:"obsPassword"`
	AutoStart        bool            `json:"autoStart"`
	Features         FeatureSettings `json:"features"`
	// IsDisableGiftGroup merges a burst of gifts from the same user into a
	// single action run (原版「禁用组刷」).
	IsDisableGiftGroup bool `json:"isDisableGiftGroup"`
	// IsOrder runs one rule's action list at a time across all rules.
	IsOrder bool `json:"isOrder"`
	// IsDisableOBS blocks every OBS WebSocket call while enabled.
	IsDisableOBS bool `json:"isDisableOBS"`
	// DebugSleepMS is the wait before a hotkey-triggered debug run.
	DebugSleepMS int `json:"debugSleepMs"`
	// Mouse movement steps; 0 moves the pointer in one jump.
	MoveTopBottomStep int `json:"moveTopBottomStep"`
	MoveLeftRightStep int `json:"moveLeftRightStep"`
	// WidgetLayouts 保存组件窗里各组件的位置与大小（拖动 / 拉伸后写回，key 是功能 ID）。
	WidgetLayouts map[string]WidgetLayout `json:"widgetLayouts,omitempty"`
}

type LicenseAuthStatus struct {
	LoggedIn    bool     `json:"loggedIn"`
	Mode        string   `json:"mode"`
	Platform    string   `json:"platform"`
	ExpiresAt   string   `json:"expiresAt,omitempty"`
	Features    []string `json:"features"`
	SafeCodeSet bool     `json:"safeCodeSet,omitempty"`
}

type LogEntry struct {
	ID       int64  `json:"id"`
	Level    string `json:"level"`
	Category string `json:"category"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
	TS       int64  `json:"ts"`
}

type ExportConfig struct {
	SchemaVersion int             `json:"schema_version"`
	AppVersion    string          `json:"app_version"`
	Rules         []Rule          `json:"rules"`
	Assets        []string        `json:"assets"`
	Overlay       OverlayExport   `json:"overlay"`
	Slot          map[string]any  `json:"slot"`
	Connectors    ConnectorExport `json:"connectors"`
	Danmaku       DanmakuExport   `json:"danmaku"`
	Features      FeatureSettings `json:"features"`
}

type OverlayExport struct {
	Green OverlaySettings `json:"green"`
	Slot  OverlaySettings `json:"slot"`
}

type ConnectorExport struct {
	Platform Platform `json:"platform"`
	RoomID   string   `json:"roomId"`
}

type DanmakuExport struct {
	RetentionDays    int  `json:"retention_days"`
	RetentionMaxRows int  `json:"retention_max_rows"`
	StoreRaw         bool `json:"store_raw"`
}

type DanmakuExportRequest struct {
	Filter DanmakuFilter `json:"filter"`
	Format string        `json:"format"`
}

type DanmakuExportResult struct {
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

type OperationResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type OperationResultWithAuth struct {
	LoggedIn bool   `json:"loggedIn"`
	Message  string `json:"message"`
}

type ConfigImportResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Rules   []Rule `json:"rules,omitempty"`
}

type OverlayMessage struct {
	Target  string `json:"target"`
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type RuleOutcome struct {
	RuleID   string `json:"ruleId,omitempty"`
	RuleName string `json:"ruleName,omitempty"`
	Result   string `json:"result"`
	Reason   string `json:"reason,omitempty"`
}

func nowMillis() int64 { return time.Now().UnixMilli() }
