package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const applicationVersion = "0.2.0"

type AppService struct {
	mu              sync.Mutex
	screenLockMu    sync.Mutex
	settingsMu      sync.RWMutex
	authMu          sync.RWMutex
	app             *application.App
	store           *Store
	appDir          string
	assetsRoot      string
	version         string
	mainWindow      *application.WebviewWindow
	greenWindow     *application.WebviewWindow
	slotWindow      *application.WebviewWindow
	audioWindow     *application.WebviewWindow
	overlayReady    map[string]chan struct{}
	overlayReadySet map[string]bool
	overlayCreating map[string]chan struct{}
	audioReady      chan struct{}
	audioReadySet   bool
	audioCreating   chan struct{}
	overlayModes    map[string]string
	settings        AppSettings
	installID       string
	license         *licenseClient
	auth            LicenseAuthStatus
	authExpiresAt   time.Time
	connection      ConnectorStatus
	obs             *obsClient
	slotWidgets     map[FeatureID]OverlayWidgetPayload
	featureCooldown map[FeatureID]int64
	widgetIDs       map[FeatureID]string
	ruleStateMu     sync.Mutex
	ruleNextAllowed map[string]int64
	ruleActive      map[string]int
	ruleLocks       map[string]*sync.Mutex
	ruleTriggerLast map[string]int64
	ruleHotkeys     []string
	orderMu         sync.Mutex
	rulesEnabled    bool
	eventsPaused    bool
	eventMu         sync.Mutex
	eventCond       *sync.Cond
	eventQueue      []queuedLiveEvent
	eventDedupe     map[string]int64
	eventStopping   bool
	eventCtx        context.Context
	eventCancel     context.CancelFunc
	eventWorkers    sync.WaitGroup
	serialMu        sync.Mutex
	serialPulses    map[*serialPulseRun]struct{}
	serialPortLocks map[string]*sync.Mutex
	connMu          sync.Mutex
	connector       *connectorRun
	// screenLockKeyOn 记录锁链的全局空格键观察器是否已生效。
	screenLockKeyMu sync.Mutex
	screenLockKeyOn bool
	// giftIcons 是「礼物名称 → 图标地址」表，由平台连接器在拉到礼物表后写入。
	giftIconMu sync.Mutex
	giftIcons  map[string]string
	closing    bool
}

type AuthLoginPayload struct {
	Platform Platform `json:"platform"`
	Code     string   `json:"code"`
	Local    bool     `json:"local"`
}

type DanmakuClearRequest struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

func NewAppService(store *Store, appDir, version string) (*AppService, error) {
	if version == "" {
		version = applicationVersion
	}
	s := &AppService{
		store: store, appDir: appDir, assetsRoot: filepath.Join(appDir, "assets"), version: version,
		license: newLicenseClient(), obs: newOBSClient(), slotWidgets: map[FeatureID]OverlayWidgetPayload{},
		overlayReady: map[string]chan struct{}{}, overlayReadySet: map[string]bool{}, overlayCreating: map[string]chan struct{}{},
		overlayModes:    map[string]string{"green": "green", "slot": "landscape-16-9"},
		featureCooldown: map[FeatureID]int64{}, widgetIDs: map[FeatureID]string{},
		ruleNextAllowed: map[string]int64{}, ruleActive: map[string]int{}, ruleLocks: map[string]*sync.Mutex{},
		ruleTriggerLast: map[string]int64{}, rulesEnabled: true,
		connection: ConnectorStatus{Platform: string(PlatformSimulator), State: "disconnected", Mode: "simulator"},
		auth:       LicenseAuthStatus{LoggedIn: false, Mode: "remote", Platform: string(PlatformSimulator), Features: []string{}},
	}
	if localDevelopmentModeAllowed() {
		s.auth = LicenseAuthStatus{LoggedIn: true, Mode: "local", Platform: string(PlatformSimulator), Features: []string{"overlay", "slot", "serial"}}
	}
	s.settings = defaultAppSettings(version, s.assetsRoot)
	if err := store.GetSetting(context.Background(), "settings", s.settings, &s.settings); err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	s.settings = mergeAppSettings(s.settings, version, s.assetsRoot)
	// 绿幕窗口与组件窗口不跨进程保留：每次启动都把它们置为「关闭」，
	// 免得上一轮运行时（播放视频、输出组件会自动打开窗口）留下来的打开状态
	// 被当成“窗口还开着”，让扩展页与设置页显示成已打开。
	s.resetOverlayWindows()
	if err := store.GetSetting(context.Background(), "authorizationInstallId", "", &s.installID); err != nil {
		return nil, fmt.Errorf("load installation id: %w", err)
	}
	if s.installID == "" {
		var id [24]byte
		if _, err := rand.Read(id[:]); err != nil {
			return nil, fmt.Errorf("create installation id: %w", err)
		}
		s.installID = hex.EncodeToString(id[:])
		if err := store.SetSetting(context.Background(), "authorizationInstallId", s.installID); err != nil {
			return nil, fmt.Errorf("save installation id: %w", err)
		}
	}
	if _, err := store.CleanupRetention(context.Background(), s.settings.RetentionDays, s.settings.RetentionMaxRows); err != nil {
		return nil, fmt.Errorf("apply record retention: %w", err)
	}
	if err := s.persistSettings(); err != nil {
		return nil, err
	}
	s.eventDedupe = make(map[string]int64)
	s.eventCond = sync.NewCond(&s.eventMu)
	s.eventCtx, s.eventCancel = context.WithCancel(context.Background())
	s.startEventWorkers()
	s.log("info", "app", "应用已启动", "version="+version+" mode=wails")
	return s, nil
}

func (s *AppService) bind(app *application.App) { s.app = app }
func (s *AppService) setMainWindow(window *application.WebviewWindow) {
	s.mainWindow = window
	if err := s.app.GlobalShortcut.Register("CmdOrCtrl+F1", func() {
		status, err := s.OverlayToggleOpacity("slot")
		if err != nil {
			s.log("warn", "hotkey", "组件窗底板快捷键执行失败", err.Error())
			return
		}
		s.log("info", "hotkey", "组件窗底板已切换", fmt.Sprintf("transparent=%t", status.BackgroundTransparent))
	}); err != nil {
		s.log("warn", "hotkey", "无法注册组件窗底板快捷键", err.Error())
	}
	for i := 1; i <= 9; i++ {
		index := i - 1
		shortcut := fmt.Sprintf("CmdOrCtrl+Shift+%d", i)
		if s.debugKeysActive() {
			if err := s.app.GlobalShortcut.Register(shortcut, func() { s.debugRuleByIndex(index) }); err != nil {
				s.log("warn", "hotkey", "无法注册规则调试快捷键", shortcut+": "+err.Error())
			}
		}
	}
	// 开启/停止 and 暂停/继续 are fixed hotkeys in 通用设置 and cannot be edited.
	if err := s.app.GlobalShortcut.Register(rulesToggleAccelerator, func() { s.toggleRulesEnabled() }); err != nil {
		s.log("warn", "hotkey", "无法注册开启/停止快捷键", rulesToggleAccelerator+": "+err.Error())
	}
	if err := s.app.GlobalShortcut.Register(rulesPauseAccelerator, func() { s.toggleEventsPaused() }); err != nil {
		s.log("warn", "hotkey", "无法注册暂停/继续快捷键", rulesPauseAccelerator+": "+err.Error())
	}
	s.applyRuleHotkeys()
}

// Fixed 通用设置 hotkeys. The labels are what the settings page shows; the
// accelerators are what Wails registers.
const (
	rulesToggleAccelerator = "CmdOrCtrl+0"
	rulesToggleKeyLabel    = "Ctrl + 0"
	rulesPauseAccelerator  = "CmdOrCtrl+F12"
	rulesPauseKeyLabel     = "Ctrl + F12"
	overlayToggleKeyLabel  = "Ctrl + F1"
)

func (s *AppService) debugKeysActive() bool {
	return s.settingSnapshot().DevMode && localDevelopmentModeAllowed()
}

func (s *AppService) rulesEngineEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rulesEnabled
}

func (s *AppService) eventsArePaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eventsPaused
}

func (s *AppService) toggleRulesEnabled() {
	s.RulesSetEnabled(!s.rulesEngineEnabled())
}
func (s *AppService) toggleEventsPaused() {
	s.RulesSetPaused(!s.eventsArePaused())
}

// RulesRuntimeState reports the footer switch of the control centre.
func (s *AppService) RulesRuntimeState() RulesRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return RulesRuntimeState{Enabled: s.rulesEnabled, Paused: s.eventsPaused, DebugKeys: s.debugKeysActive(), OpenKey: rulesToggleKeyLabel, CloseKey: rulesPauseKeyLabel}
}

// RulesSetEnabled is the global 开启/停止 switch: events keep being recorded but
// no rule or feature action runs while it is off.
func (s *AppService) RulesSetEnabled(enabled bool) RulesRuntimeState {
	s.mu.Lock()
	s.rulesEnabled = enabled
	s.mu.Unlock()
	if enabled {
		s.log("info", "rules", "规则引擎已开启", "")
	} else {
		s.log("warn", "rules", "规则引擎已停止", "事件仍会记录，但不会执行任何动作")
	}
	return s.emitRulesState()
}

// RulesSetPaused is the 暂停/继续 switch: paused events never enter the queue.
func (s *AppService) RulesSetPaused(paused bool) RulesRuntimeState {
	s.mu.Lock()
	s.eventsPaused = paused
	s.mu.Unlock()
	if paused {
		s.log("warn", "rules", "事件处理已暂停", "暂停期间新事件不会入队")
	} else {
		s.log("info", "rules", "事件处理已继续", "")
	}
	return s.emitRulesState()
}

func (s *AppService) emitRulesState() RulesRuntimeState {
	state := s.RulesRuntimeState()
	s.emit("rules:state", state)
	return state
}

// applyRuleHotkeys re-registers every rule that binds a hotkey. Rules without a
// hotkey are simply skipped.
func (s *AppService) applyRuleHotkeys() {
	if s.app == nil {
		return
	}
	for _, accelerator := range s.ruleHotkeys {
		if err := s.app.GlobalShortcut.Unregister(accelerator); err != nil {
			s.log("warn", "hotkey", "无法解除玩法热键", accelerator+": "+err.Error())
		}
	}
	s.ruleHotkeys = nil
	rules, err := s.store.ListRules(context.Background())
	if err != nil {
		s.log("warn", "hotkey", "读取玩法热键失败", err.Error())
		return
	}
	for _, rule := range rules {
		accelerator, err := ruleHotkeyAccelerator(rule.Hotkey)
		if err != nil {
			if strings.TrimSpace(rule.Hotkey) != "" {
				s.log("warn", "hotkey", "玩法热键无效", rule.Name+": "+err.Error())
			}
			continue
		}
		ruleID := rule.ID
		if err := s.app.GlobalShortcut.Register(accelerator, func() { s.triggerRuleByHotkey(ruleID) }); err != nil {
			s.log("warn", "hotkey", "无法注册玩法热键", rule.Name+": "+err.Error())
			continue
		}
		s.ruleHotkeys = append(s.ruleHotkeys, accelerator)
	}
}

func (s *AppService) triggerRuleByHotkey(id string) {
	result := s.RulesTrigger(id)
	if !result.OK {
		s.log("warn", "hotkey", "玩法热键触发失败", result.Message)
	}
}

// ruleHotkeyAccelerator converts the recorder format ("Ctrl + Shift + F") into a
// Wails accelerator. Only the keys Wails can register are accepted: A-Z, 0-9
// and F1-F12, optionally combined with Shift / Ctrl / Alt.
func ruleHotkeyAccelerator(hotkey string) (string, error) {
	hotkey = strings.TrimSpace(hotkey)
	if hotkey == "" {
		return "", nil
	}
	parts := strings.Split(hotkey, "+")
	modifiers := make([]string, 0, 3)
	key := ""
	seen := map[string]bool{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			return "", errors.New("热键格式无效")
		}
		switch strings.ToLower(item) {
		case "ctrl", "control":
			if !seen["ctrl"] {
				seen["ctrl"] = true
				modifiers = append(modifiers, "CmdOrCtrl")
			}
		case "shift":
			if !seen["shift"] {
				seen["shift"] = true
				modifiers = append(modifiers, "Shift")
			}
		case "alt":
			if !seen["alt"] {
				seen["alt"] = true
				modifiers = append(modifiers, "Alt")
			}
		default:
			if key != "" {
				return "", errors.New("热键只能包含一个主键")
			}
			upper := strings.ToUpper(item)
			switch {
			case len(upper) == 1 && (upper[0] >= 'A' && upper[0] <= 'Z' || upper[0] >= '0' && upper[0] <= '9'):
			case strings.HasPrefix(upper, "F") && isNumeric(upper[1:]) && len(upper) <= 3:
				if number := atoiSafe(upper[1:]); number < 1 || number > 12 {
					return "", errors.New("只支持 F1 到 F12")
				}
			default:
				return "", fmt.Errorf("不支持的按键：%s", item)
			}
			key = upper
		}
	}
	if key == "" {
		return "", errors.New("热键需要包含一个主键（字母、数字或 F1-F12）")
	}
	if len(modifiers) == 0 {
		return "", errors.New("热键至少需要一个修饰键（Ctrl / Shift / Alt）")
	}
	return strings.Join(append(modifiers, key), "+"), nil
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func atoiSafe(value string) int {
	number := 0
	for _, char := range value {
		number = number*10 + int(char-'0')
	}
	return number
}

// validateRuleHotkey rejects hotkeys that are reserved or already bound.
func (s *AppService) validateRuleHotkey(rule Rule) error {
	if strings.TrimSpace(rule.Hotkey) == "" {
		return nil
	}
	accelerator, err := ruleHotkeyAccelerator(rule.Hotkey)
	if err != nil {
		return err
	}
	switch accelerator {
	case "CmdOrCtrl+F1":
		return fmt.Errorf("热键 %s 已被组件窗底板切换占用", overlayToggleKeyLabel)
	case rulesToggleAccelerator:
		return fmt.Errorf("热键 %s 已被开启/停止占用", rulesToggleKeyLabel)
	case rulesPauseAccelerator:
		return fmt.Errorf("热键 %s 已被暂停/继续占用", rulesPauseKeyLabel)
	}
	rules, err := s.store.ListRules(context.Background())
	if err != nil {
		return err
	}
	for _, other := range rules {
		if other.ID == rule.ID {
			continue
		}
		otherAccelerator, err := ruleHotkeyAccelerator(other.Hotkey)
		if err != nil || otherAccelerator != accelerator {
			continue
		}
		return fmt.Errorf("热键已被「%s」占用", other.Name)
	}
	return nil
}

func (s *AppService) shutdown() {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	s.closing = true
	s.mu.Unlock()
	s.stopConnector()
	s.stopEventWorkers()
	stopScreenLockKeyObserver()
	s.app.GlobalShortcut.UnregisterAll()
	s.SerialStopAll()
	s.obs.Disconnect()
	if err := s.persistSettings(); err != nil {
		s.log("warn", "store", "应用关闭时保存设置失败", err.Error())
	}
	_ = s.store.Close()
}

func (s *AppService) RulesList() ([]Rule, error) { return s.store.ListRules(context.Background()) }
func (s *AppService) RulesSave(rule Rule) (Rule, error) {
	if rule.ID == "" {
		rule.ID = randomID()
	}
	if strings.TrimSpace(rule.Name) == "" {
		rule.Name = "未命名玩法"
	} else {
		rule.Name = strings.TrimSpace(rule.Name)
	}
	if len(rule.Trigger.Kinds) == 0 {
		rule.Trigger.Kinds = []EventKind{KindGift}
	}
	if rule.Concurrency == "" {
		rule.Concurrency = "queue"
	}
	if rule.Actions == nil {
		rule.Actions = []Action{}
	}
	rule.Hotkey = strings.TrimSpace(rule.Hotkey)
	if err := s.validateRuleHotkey(rule); err != nil {
		return Rule{}, err
	}
	saved, err := s.store.SaveRule(context.Background(), rule)
	if err == nil {
		s.log("info", "rules", "已保存规则", saved.Name)
		s.applyRuleHotkeys()
	}
	return saved, err
}
func (s *AppService) RulesRemove(id string) error {
	if err := s.store.RemoveRule(context.Background(), id); err != nil {
		return err
	}
	s.applyRuleHotkeys()
	return nil
}
func (s *AppService) RulesClear() error {
	if err := s.store.ClearRules(context.Background()); err != nil {
		return err
	}
	s.applyRuleHotkeys()
	return nil
}

// RulesSetPinned implements the control centre 置顶 row action.
func (s *AppService) RulesSetPinned(id string, pinned bool) (Rule, error) {
	rules, err := s.store.ListRules(context.Background())
	if err != nil {
		return Rule{}, err
	}
	for _, rule := range rules {
		if rule.ID != id {
			continue
		}
		rule.Pinned = pinned
		return s.store.SaveRule(context.Background(), rule)
	}
	return Rule{}, errors.New("规则不存在")
}

// RulesClone copies a rule `count` times; the copies start disabled so a clone
// never fires before it is reviewed.
func (s *AppService) RulesClone(id string, count int) ([]Rule, error) {
	count = min(max(count, 1), 50)
	rules, err := s.store.ListRules(context.Background())
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if rule.ID != id {
			continue
		}
		clones := make([]Rule, 0, count)
		for index := 0; index < count; index++ {
			clone := rule
			clone.ID, clone.Enabled, clone.Pinned, clone.Hotkey = randomID(), false, false, ""
			suffix := " - 副本"
			if count > 1 {
				suffix = fmt.Sprintf(" - 副本%d", index+1)
			}
			clone.Name = rule.Name + suffix
			saved, err := s.store.SaveRule(context.Background(), clone)
			if err != nil {
				return clones, err
			}
			clones = append(clones, saved)
		}
		return clones, nil
	}
	return nil, errors.New("规则不存在")
}

// RulesTrigger executes a rule's action list right away, without waiting for a
// matching event. It backs the per-rule hotkey and the 调试 action.
func (s *AppService) RulesTrigger(id string) OperationResult {
	rules, err := s.store.ListRules(context.Background())
	if err != nil {
		return OperationResult{Message: err.Error()}
	}
	for _, rule := range rules {
		if rule.ID != id {
			continue
		}
		if !s.rulesEngineEnabled() {
			return OperationResult{Message: "规则引擎已停止（" + rulesToggleKeyLabel + " 可恢复）"}
		}
		if sleep := s.settingSnapshot().DebugSleepMS; sleep > 0 {
			if !waitForContext(context.Background(), time.Duration(sleep)*time.Millisecond) {
				return OperationResult{Message: "调试延时等待已取消"}
			}
		}
		var queueLock *sync.Mutex
		if rule.Concurrency == "queue" && !rule.Nowait {
			queueLock = s.ruleMutex(rule.ID)
			queueLock.Lock()
		}
		outcome := s.executeRuleActions(context.Background(), rule, debugEventForRule(rule))
		if queueLock != nil {
			queueLock.Unlock()
		}
		if outcome.Result == "failed" {
			return OperationResult{Message: outcome.Reason}
		}
		s.log("info", "rule", fmt.Sprintf("[rule:%s] 手动触发", rule.Name), "")
		return OperationResult{OK: true, Message: "已执行「" + rule.Name + "」"}
	}
	return OperationResult{Message: "规则不存在"}
}

func (s *AppService) DanmakuQuery(filter DanmakuFilter) ([]DanmakuRecord, error) {
	return s.store.QueryRecords(context.Background(), filter)
}
func (s *AppService) DanmakuCount(filter DanmakuFilter) (int, error) {
	return s.store.CountRecords(context.Background(), filter)
}
func (s *AppService) DanmakuClear(filter DanmakuClearRequest) (int64, error) {
	count, err := s.store.ClearRecords(context.Background(), filter.From, filter.To)
	if err == nil {
		s.log("info", "danmaku", "已清除弹幕记录", fmt.Sprintf("count=%d", count))
	}
	return count, err
}
func (s *AppService) DanmakuExport(filter DanmakuFilter, format string) (DanmakuExportResult, error) {
	if format != "json" && format != "csv" && format != "txt" {
		return DanmakuExportResult{}, errors.New("不支持的导出格式")
	}
	filter.Limit = min(max(filter.Limit, 1), 500)
	records, err := s.store.QueryRecords(context.Background(), filter)
	if err != nil {
		return DanmakuExportResult{}, err
	}
	content, err := formatRecords(records, format)
	if err != nil {
		return DanmakuExportResult{}, err
	}
	dialog := s.app.Dialog.SaveFile()
	dialog.SetOptions(&application.SaveFileDialogOptions{Title: "导出弹幕记录", Filename: fmt.Sprintf("danmaku-%d.%s", nowMillis(), format), Filters: []application.FileFilter{{DisplayName: strings.ToUpper(format), Pattern: "*." + format}}, Window: s.mainWindow})
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return DanmakuExportResult{}, err
	}
	if path == "" {
		return DanmakuExportResult{Content: content}, nil
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return DanmakuExportResult{}, err
	}
	s.log("info", "danmaku", "已导出弹幕记录", fmt.Sprintf("format=%s rows=%d", format, len(records)))
	return DanmakuExportResult{Path: path}, nil
}

func (s *AppService) ConnConnect(platform Platform, roomID string) (ConnectorStatus, error) {
	if !containsPlatform(platform) {
		return ConnectorStatus{}, errors.New("不支持的平台")
	}
	if !s.authorized("") {
		return ConnectorStatus{}, errors.New("请先验证卡密")
	}
	roomID = normalizeRoomInput(roomID)
	if roomID == "" {
		return ConnectorStatus{}, errors.New("无法识别直播间号：请填写直播间号、分享链接（v.douyin.com/xxxx）或直播间链接")
	}
	s.settingsMu.Lock()
	s.settings.Platform, s.settings.RoomID = platform, roomID
	s.settingsMu.Unlock()
	if err := s.persistSettings(); err != nil {
		return ConnectorStatus{}, err
	}
	s.mu.Lock()
	s.connection.Platform, s.connection.RoomID = string(platform), roomID
	s.connection.State, s.connection.Mode = connectorStateConnecting, connectorMode(platform)
	s.connection.Reconnects, s.connection.Dropped, s.connection.LastEventAt = 0, 0, 0
	s.connection.LastError = ""
	status := s.connection
	s.mu.Unlock()
	s.emit("conn:status", status)
	s.log("info", "connector", "正在连接直播间", platformLabel(platform)+" "+roomID)
	s.startConnector(platform, roomID)
	return status, nil
}

func (s *AppService) ConnDisconnect() ConnectorStatus {
	s.stopConnector()
	s.mu.Lock()
	s.connection.State, s.connection.LastError = connectorStateDisconnected, ""
	status := s.connection
	s.mu.Unlock()
	s.emit("conn:status", status)
	s.log("info", "connector", "已断开直播间连接", "")
	return status
}
func (s *AppService) ConnStatus() ConnectorStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connection
}
func (s *AppService) ConnSimulate(input LiveEvent) (LiveEvent, error) {
	if !s.authorized("") {
		return LiveEvent{}, errors.New("请先验证卡密")
	}
	if input.Kind == "" {
		return LiveEvent{}, errors.New("事件类型无效")
	}
	if input.ID == "" {
		input.ID = randomID()
	}
	if input.Source == "" {
		input.Source = s.ConnStatus().Platform
	}
	if input.RoomID == "" {
		input.RoomID = s.ConnStatus().RoomID
	}
	if input.Timestamp == 0 {
		input.Timestamp = nowMillis()
	}
	if input.User == nil {
		input.User = &LiveUser{Name: "模拟观众"}
	}
	s.enqueueEvent(input)
	return input, nil
}

func (s *AppService) AuthStatus() LicenseAuthStatus {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if s.auth.Mode == "remote" && s.auth.LoggedIn {
		remote, err := s.license.Status(s.settingSnapshot().AuthServerURL)
		if err != nil {
			s.auth = LicenseAuthStatus{LoggedIn: false, Mode: "remote", Platform: s.auth.Platform, Features: []string{}}
			s.authExpiresAt = time.Time{}
		} else {
			s.applyRemoteAuth(remote)
		}
	}
	return cloneAuthStatus(s.auth)
}
func (s *AppService) AuthDevelopmentModeAvailable() bool { return localDevelopmentModeAllowed() }

func (s *AppService) AuthLogin(payload AuthLoginPayload) OperationResultWithAuth {
	if !containsPlatform(payload.Platform) {
		return OperationResultWithAuth{LoggedIn: false, Message: "登录参数无效"}
	}
	if !payload.Local && strings.TrimSpace(s.settingSnapshot().AuthServerURL) == "" {
		return OperationResultWithAuth{LoggedIn: false, Message: "正式授权服务尚未配置，请联系管理员"}
	}
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if payload.Local {
		if !localDevelopmentModeAllowed() {
			return OperationResultWithAuth{LoggedIn: false, Message: "正式版本不开放本地开发模式"}
		}
		_ = s.license.Logout()
		s.auth = LicenseAuthStatus{LoggedIn: true, Mode: "local", Platform: string(payload.Platform), Features: []string{"overlay", "slot", "serial"}}
		s.authExpiresAt = time.Time{}
		s.log("info", "auth", "已进入本地开发模式", "platform="+string(payload.Platform))
		return OperationResultWithAuth{LoggedIn: true, Message: "已进入本地开发模式"}
	}
	remote, err := s.license.Login(s.settingSnapshot().AuthServerURL, payload.Code, payload.Platform, s.installID)
	if err != nil {
		s.auth = LicenseAuthStatus{LoggedIn: false, Mode: "remote", Platform: string(payload.Platform), Features: []string{}}
		s.authExpiresAt = time.Time{}
		s.log("warn", "auth", "卡密验证失败", err.Error())
		return OperationResultWithAuth{LoggedIn: false, Message: err.Error()}
	}
	s.applyRemoteAuth(remote)
	s.settingsMu.Lock()
	s.settings.Platform = payload.Platform
	s.settingsMu.Unlock()
	if err := s.persistSettings(); err != nil {
		s.log("warn", "auth", "卡密验证成功，但直播平台设置未能保存", err.Error())
		return OperationResultWithAuth{LoggedIn: true, Message: "卡密验证成功，但直播平台设置未能保存：" + err.Error()}
	}
	s.log("info", "auth", "卡密验证成功", "platform="+string(payload.Platform)+" features="+strings.Join(remote.Features, ","))
	return OperationResultWithAuth{LoggedIn: true, Message: "卡密验证成功，已建立短期授权会话"}
}
func (s *AppService) AuthLogout() error {
	s.authMu.Lock()
	err := s.license.Logout()
	s.auth = LicenseAuthStatus{LoggedIn: false, Mode: "remote", Platform: s.auth.Platform, Features: []string{}}
	s.authExpiresAt = time.Time{}
	s.authMu.Unlock()
	s.log("info", "auth", "已退出授权", "")
	return err
}
func (s *AppService) AuthSetSafeCode(code string) OperationResult {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if err := s.license.SetSafeCode(s.settingSnapshot().AuthServerURL, code); err != nil {
		return OperationResult{Message: err.Error()}
	}
	s.auth.SafeCodeSet = true
	return OperationResult{OK: true, Message: "安全码已保存"}
}
func (s *AppService) AuthUnbind(safeCode string) OperationResult {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if err := s.license.Unbind(s.settingSnapshot().AuthServerURL, safeCode); err != nil {
		return OperationResult{Message: err.Error()}
	}
	s.auth = LicenseAuthStatus{LoggedIn: false, Mode: "remote", Platform: s.auth.Platform, Features: []string{}}
	s.authExpiresAt = time.Time{}
	return OperationResult{OK: true, Message: "本机设备绑定已解除"}
}

func (s *AppService) DiagnosticsLogs(limit int) ([]LogEntry, error) {
	return s.store.ListLogs(context.Background(), limit)
}
func (s *AppService) DiagnosticsAssets() ([]string, error) {
	items := make([]string, 0)
	err := filepath.WalkDir(s.assetsRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if strings.Contains("|.png|.jpg|.jpeg|.bmp|.gif|.svg|.webp|.mp3|.wav|.mp4|.webm|.mov|.m4v|", "|"+ext+"|") {
			rel, err := filepath.Rel(s.assetsRoot, path)
			if err != nil {
				return err
			}
			items = append(items, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(items)
	return items, err
}
func (s *AppService) DiagnosticsSettings() AppSettings {
	settings := s.settingSnapshot()
	if settings.OBSPassword != "" {
		settings.OBSPassword = "***"
	}
	return settings
}
func (s *AppService) DiagnosticsSaveSettings(patch map[string]any) (AppSettings, error) {
	s.settingsMu.Lock()
	merged, err := mergeSettingsPatch(s.settings, patch)
	if err == nil {
		s.settings = merged
	}
	s.settingsMu.Unlock()
	if err != nil {
		return AppSettings{}, err
	}
	if err := s.persistSettings(); err != nil {
		return AppSettings{}, err
	}
	s.applyOverlaySettings("green", merged.OverlayGreen)
	s.applyOverlaySettings("slot", merged.OverlaySlot)
	return s.DiagnosticsSettings(), nil
}

func (s *AppService) ConfigExport() OperationResult {
	config, err := s.buildExportConfig()
	if err != nil {
		return OperationResult{Message: err.Error()}
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return OperationResult{Message: err.Error()}
	}
	dialog := s.app.Dialog.SaveFile()
	dialog.SetOptions(&application.SaveFileDialogOptions{Title: "导出配置", Filename: "abi-livetool-config.json", Filters: []application.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}}, Window: s.mainWindow})
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return OperationResult{Message: err.Error()}
	}
	if path == "" {
		return OperationResult{Message: "已取消导出"}
	}
	if _, err := os.Stat(path); err == nil {
		old, readErr := os.ReadFile(path)
		if readErr != nil {
			return OperationResult{Message: readErr.Error()}
		}
		if err := os.WriteFile(path+".bak", old, 0o600); err != nil {
			return OperationResult{Message: err.Error()}
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return OperationResult{Message: err.Error()}
	}
	s.log("info", "config", "配置已导出", filepath.Base(path))
	return OperationResult{OK: true, Message: "配置导出成功", Path: path}
}
func (s *AppService) ConfigImport() ConfigImportResult {
	dialog := s.app.Dialog.OpenFile()
	dialog.SetOptions(&application.OpenFileDialogOptions{Title: "导入配置", CanChooseFiles: true, CanChooseDirectories: false, Filters: []application.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}}, Window: s.mainWindow})
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return ConfigImportResult{Message: err.Error()}
	}
	if path == "" {
		return ConfigImportResult{Message: "已取消导入"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigImportResult{Message: err.Error()}
	}
	var config ExportConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ConfigImportResult{Message: "配置不是有效 JSON"}
	}
	type importedOverlay struct {
		Green map[string]any `json:"green"`
		Slot  map[string]any `json:"slot"`
	}
	type importedDanmaku struct {
		RetentionDays    *int  `json:"retention_days"`
		RetentionMaxRows *int  `json:"retention_max_rows"`
		StoreRaw         *bool `json:"store_raw"`
	}
	var importedFields struct {
		Overlay importedOverlay  `json:"overlay"`
		Danmaku *importedDanmaku `json:"danmaku"`
	}
	if err := json.Unmarshal(data, &importedFields); err != nil {
		return ConfigImportResult{Message: "配置不是有效 JSON"}
	}
	if config.SchemaVersion != 1 || config.Rules == nil {
		return ConfigImportResult{Message: "不支持的配置版本或缺少 rules"}
	}
	for _, rule := range config.Rules {
		if _, err := s.RulesSave(rule); err != nil {
			return ConfigImportResult{Message: err.Error()}
		}
	}
	s.settingsMu.Lock()
	if importedFields.Overlay.Green != nil {
		updated, err := mergeOverlaySettings(s.settings.OverlayGreen, importedFields.Overlay.Green)
		if err != nil {
			s.settingsMu.Unlock()
			return ConfigImportResult{Message: err.Error()}
		}
		s.settings.OverlayGreen = updated
	}
	if importedFields.Overlay.Slot != nil {
		updated, err := mergeOverlaySettings(s.settings.OverlaySlot, importedFields.Overlay.Slot)
		if err != nil {
			s.settingsMu.Unlock()
			return ConfigImportResult{Message: err.Error()}
		}
		s.settings.OverlaySlot = updated
	}
	if config.Connectors.Platform != "" {
		s.settings.Platform = config.Connectors.Platform
		if config.Connectors.RoomID != "" {
			s.settings.RoomID = config.Connectors.RoomID
		}
	}
	if importedFields.Danmaku != nil {
		if importedFields.Danmaku.RetentionDays != nil {
			s.settings.RetentionDays = *importedFields.Danmaku.RetentionDays
		}
		if importedFields.Danmaku.RetentionMaxRows != nil {
			s.settings.RetentionMaxRows = *importedFields.Danmaku.RetentionMaxRows
		}
		if importedFields.Danmaku.StoreRaw != nil {
			s.settings.StoreRaw = *importedFields.Danmaku.StoreRaw
		}
	}
	if config.Features != nil {
		s.settings.Features = mergeFeatureSettings(config.Features)
	}
	settings := s.settings
	s.settingsMu.Unlock()
	s.applyOverlaySettings("green", settings.OverlayGreen)
	s.applyOverlaySettings("slot", settings.OverlaySlot)
	if err := s.persistSettings(); err != nil {
		return ConfigImportResult{Message: err.Error()}
	}
	rules, err := s.RulesList()
	if err != nil {
		return ConfigImportResult{Message: err.Error()}
	}
	s.log("info", "config", "配置已导入", fmt.Sprintf("rules=%d", len(config.Rules)))
	return ConfigImportResult{OK: true, Message: fmt.Sprintf("已导入 %d 条规则", len(config.Rules)), Rules: rules}
}

func (s *AppService) WindowMinimize() {
	if s.mainWindow != nil {
		s.mainWindow.Minimise()
	}
}
func (s *AppService) WindowMaximize() {
	if s.mainWindow != nil {
		s.mainWindow.ToggleMaximise()
	}
}
func (s *AppService) WindowClose() {
	if s.mainWindow != nil {
		s.mainWindow.Close()
	} else if s.app != nil {
		s.app.Quit()
	}
}

func (s *AppService) settingSnapshot() AppSettings {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	data, _ := json.Marshal(s.settings)
	var copy AppSettings
	_ = json.Unmarshal(data, &copy)
	return copy
}
func (s *AppService) persistSettings() error {
	return s.store.SetSetting(context.Background(), "settings", s.settingSnapshot())
}
func (s *AppService) log(level, category, message, detail string) {
	entry := LogEntry{Level: level, Category: category, Message: message, Detail: detail, TS: nowMillis()}
	if id, err := s.store.SaveLog(context.Background(), entry); err == nil {
		entry.ID = id
	}
	s.emit("log:append", entry)
}
func (s *AppService) emit(name string, payload any) {
	if s.app != nil {
		s.app.Event.Emit(name, payload)
	}
}
func (s *AppService) applyRemoteAuth(remote remoteAuthSession) {
	s.auth.LoggedIn, s.auth.Mode, s.auth.ExpiresAt = true, "remote", remote.ExpiresAt
	s.auth.Features = append([]string(nil), remote.Features...)
	s.auth.SafeCodeSet = remote.SafeCodeSet
	s.authExpiresAt, _ = time.Parse(time.RFC3339Nano, remote.SessionExpiresAt)
}
func (s *AppService) authorized(entitlement string) bool {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	if !s.auth.LoggedIn {
		return false
	}
	if s.auth.Mode == "local" {
		return localDevelopmentModeAllowed()
	}
	if s.auth.Mode != "remote" || time.Now().After(s.authExpiresAt) {
		return false
	}
	if entitlement == "" {
		return true
	}
	for _, feature := range s.auth.Features {
		if feature == entitlement {
			return true
		}
	}
	return false
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

var _ = hex.EncodeToString
