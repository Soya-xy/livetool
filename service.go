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
	closing         bool
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
		if s.settings.DevMode && localDevelopmentModeAllowed() {
			if err := s.app.GlobalShortcut.Register(shortcut, func() { s.debugRuleByIndex(index) }); err != nil {
				s.log("warn", "hotkey", "无法注册规则调试快捷键", shortcut+": "+err.Error())
			}
		}
	}
}

func (s *AppService) shutdown() {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	s.closing = true
	s.mu.Unlock()
	s.stopEventWorkers()
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
	saved, err := s.store.SaveRule(context.Background(), rule)
	if err == nil {
		s.log("info", "rules", "已保存规则", saved.Name)
	}
	return saved, err
}
func (s *AppService) RulesRemove(id string) error {
	return s.store.RemoveRule(context.Background(), id)
}
func (s *AppService) RulesClear() error { return s.store.ClearRules(context.Background()) }
func (s *AppService) RulesClone(id string) (Rule, error) {
	rules, err := s.store.ListRules(context.Background())
	if err != nil {
		return Rule{}, err
	}
	for _, rule := range rules {
		if rule.ID == id {
			rule.ID, rule.Name, rule.Enabled = randomID(), rule.Name+" - 副本", false
			return s.store.SaveRule(context.Background(), rule)
		}
	}
	return Rule{}, errors.New("规则不存在")
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
	if platform != PlatformSimulator {
		return ConnectorStatus{}, errors.New("该平台连接器尚未实现，当前仅支持本地模拟器")
	}
	roomID = strings.TrimSpace(roomID)
	s.settingsMu.Lock()
	s.settings.Platform, s.settings.RoomID = platform, roomID
	s.settingsMu.Unlock()
	if err := s.persistSettings(); err != nil {
		return ConnectorStatus{}, err
	}
	s.mu.Lock()
	s.connection.Platform, s.connection.RoomID, s.connection.State, s.connection.Mode = string(platform), roomID, "connected", "simulator"
	s.connection.Reconnects, s.connection.Dropped, s.connection.LastEventAt = 0, 0, 0
	status := s.connection
	s.mu.Unlock()
	s.emit("conn:status", status)
	s.log("info", "connector", "已连接本地模拟事件源", string(platform)+" room="+roomID)
	return status, nil
}
func (s *AppService) ConnDisconnect() ConnectorStatus {
	s.mu.Lock()
	s.connection.State = "disconnected"
	status := s.connection
	s.mu.Unlock()
	s.emit("conn:status", status)
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
