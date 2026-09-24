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
	FeatureWoodfish         FeatureID = "electronic-woodfish"
	FeatureSlotMachine      FeatureID = "slot-machine"
	FeatureGiftScreen       FeatureID = "gift-screen"
	FeatureLottery          FeatureID = "lottery"
	FeatureCounter          FeatureID = "counter"
	FeatureGiftPool         FeatureID = "gift-pool"
	FeatureScreenLock       FeatureID = "screen-lock"
	FeatureMosquitoSlap     FeatureID = "mosquito-slap"
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
