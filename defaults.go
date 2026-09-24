package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// LicenseServerURL is set by the release build. The frontend never supplies the
// license service address; only the card key is entered by the user.
var LicenseServerURL string

func configuredLicenseServerURL() string {
	if !localDevelopmentModeAllowed() {
		return strings.TrimSpace(LicenseServerURL)
	}
	if value := strings.TrimSpace(os.Getenv("LIVETOOL_LICENSE_SERVER_URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(LicenseServerURL); value != "" {
		return value
	}
	return "http://127.0.0.1:8787"
}

func defaultAppSettings(version, assetsRoot string) AppSettings {
	return AppSettings{
		Version: version, DevMode: localDevelopmentModeAllowed(), LogLevel: "info", RetentionDays: 30, RetentionMaxRows: 200000,
		StoreRaw: false, Hotkey: "Ctrl + Shift + 1-9", AudioVolume: 0.8, AssetsRoot: assetsRoot,
		OverlayGreen: OverlaySettings{Visible: false, AlwaysOnTop: false, Opacity: 1, Width: 960, Height: 540, LaneCount: 3, Background: "#00ff00"},
		OverlaySlot:  OverlaySettings{Visible: false, AlwaysOnTop: false, Opacity: 1, BackgroundTransparent: true, Width: 960, Height: 540, LaneCount: 1, Background: "#000000"},
		Platform:     PlatformSimulator, RoomID: "demo-room", AuthServerURL: configuredLicenseServerURL(),
		OBSURL: "ws://127.0.0.1:4455", AutoStart: false, Features: defaultFeatureSettings(),
	}
}

func defaultFeatureSettings() FeatureSettings {
	settings := FeatureSettings{}
	add := func(id FeatureID, values map[string]any, giftRules bool) {
		rules := []FeatureGiftRule{}
		if giftRules {
			action := "增加"
			if id == FeatureFryingPan {
				action = "减少"
			}
			rules = append(rules, FeatureGiftRule{ID: string(id) + "-default-rule", GiftName: "啤酒", Action: action, Enabled: true, Values: map[string]any{}})
		}
		settings[id] = FeatureConfig{Enabled: false, Values: values, GiftRules: rules}
	}
	add(FeatureGreenWindow, map[string]any{"displayContent": "视频组件", "videoPath": "", "videoDurationMs": 8000, "videoLoop": false, "backgroundColor": "#00ff00", "laneCount": 3, "alwaysOnTop": false}, false)
	add(FeatureComponentWindow, map[string]any{"displayContent": "组件内容", "width": 960, "height": 540, "alwaysOnTop": false}, false)
	add(FeatureVirtualCamera, map[string]any{"deviceName": "阿比虚拟摄像头", "resolution": "1920x1080", "fps": 30}, false)
	add(FeatureSpeedCurve, map[string]any{"targetCount": 10, "durationMs": 3000, "curve": "ease-out"}, true)
	add(FeatureSpeedIba, map[string]any{"targetCount": 10, "durationMs": 3000, "batchSize": 1}, true)
	add(FeatureImpactGift, map[string]any{"imagePath": "images/平底锅.png", "count": 8, "gravity": 1800, "bounce": 0.45}, true)
	add(FeatureFryingPan, map[string]any{"displayContent": "煮播血条", "maxValue": 100, "damagePerGift": 10, "barColor": "#ff4f56", "currentValue": 100}, true)
	add(FeatureVoiceBroadcast, map[string]any{"audioPath": "voices/测试音效.mp3", "template": "{user} 送出 {gift}，数量 {count}", "chatTemplate": "{user} 说 {text}", "chatEnabled": true, "volume": 0.8, "interrupt": false}, true)
	add(FeatureDanmakuAssistant, map[string]any{"keywords": "666, 欧皇", "keywordMode": "contains", "replyTemplate": "收到 {user}", "cooldownMs": 3000}, false)
	add(FeatureLiveClock, map[string]any{"displayContent": "直播倒计时", "durationSeconds": 60, "secondsPerGift": 10, "style": "digital"}, true)
	add(FeatureWoodfish, map[string]any{"audioPath": "fruit.mp3", "meritPerClick": 1, "meritPerGift": 1, "showCounter": true, "currentMerit": 0}, true)
	add(FeatureSlotMachine, map[string]any{"theme": "default", "pool": "一等奖, 二等奖, 谢谢参与", "weights": "1, 10, 89", "durationMs": 2200, "playMusic": true, "nextStickerIndex": 0}, true)
	add(FeatureGiftScreen, map[string]any{"imagePath": "images/平底锅.png", "durationMs": 3500, "maxVisible": 20}, true)
	add(FeatureLottery, map[string]any{"pool": "一等奖, 二等奖, 谢谢参与", "weights": "1, 10, 89", "durationMs": 1700}, true)
	add(FeatureCounter, map[string]any{"displayContent": "礼物计数", "initialValue": 0, "step": 1, "currentValue": 0}, true)
	add(FeatureGiftPool, map[string]any{"pool": "啤酒, 小心心, 平底锅", "durationMs": 3500, "randomize": true, "nextStickerIndex": 0}, true)
	add(FeatureScreenLock, map[string]any{"displayContent": "请按空格解锁", "lockColor": "#e53935", "pressesPerGift": 1, "lockMediaPath": ""}, true)
	add(FeatureMosquitoSlap, map[string]any{"imagePath": "idle.png", "durationMs": 20000, "score": 1}, true)
	return settings
}

func mergeAppSettings(input AppSettings, version, assetsRoot string) AppSettings {
	defaults := defaultAppSettings(version, assetsRoot)
	if input.Version == "" {
		input.Version = version
	}
	if input.LogLevel == "" {
		input.LogLevel = defaults.LogLevel
	}
	if input.RetentionDays < 1 {
		input.RetentionDays = defaults.RetentionDays
	}
	if input.RetentionMaxRows < 1 {
		input.RetentionMaxRows = defaults.RetentionMaxRows
	}
	if input.Hotkey == "" {
		input.Hotkey = defaults.Hotkey
	}
	if input.AudioVolume < 0 || input.AudioVolume > 1 {
		input.AudioVolume = defaults.AudioVolume
	}
	if input.AssetsRoot == "" {
		input.AssetsRoot = assetsRoot
	}
	if input.OverlayGreen.Width < 1 {
		input.OverlayGreen = defaults.OverlayGreen
	}
	if input.OverlaySlot.Width < 1 {
		input.OverlaySlot = defaults.OverlaySlot
	}
	// Existing user configs may still have these windows pinned from older builds.
	input.OverlayGreen.AlwaysOnTop = false
	input.OverlaySlot.AlwaysOnTop = false
	if input.Platform == "" {
		input.Platform = defaults.Platform
	}
	if input.RoomID == "" {
		input.RoomID = defaults.RoomID
	}
	if !localDevelopmentModeAllowed() {
		// Ignore any server address left in an Electron-era config. Production
		// builds are pinned to their release endpoint and cannot be redirected
		// through the settings import or UI.
		input.AuthServerURL = defaults.AuthServerURL
	} else if input.AuthServerURL == "" {
		input.AuthServerURL = defaults.AuthServerURL
	}
	if input.OBSURL == "" {
		input.OBSURL = defaults.OBSURL
	}
	if !localDevelopmentModeAllowed() {
		input.DevMode = false
	}
	input.Features = mergeFeatureSettings(input.Features)
	return input
}

func mergeFeatureSettings(input FeatureSettings) FeatureSettings {
	defaults := defaultFeatureSettings()
	if input == nil {
		input = FeatureSettings{}
	}
	for id, fallback := range defaults {
		value, found := input[id]
		if !found {
			input[id] = fallback
			continue
		}
		if value.Values == nil {
			value.Values = map[string]any{}
		}
		for key, defaultValue := range fallback.Values {
			if _, ok := value.Values[key]; !ok {
				value.Values[key] = defaultValue
			}
		}
		if value.GiftRules == nil {
			value.GiftRules = fallback.GiftRules
		}
		if id == FeatureGreenWindow || id == FeatureComponentWindow {
			value.Values["alwaysOnTop"] = false
		}
		input[id] = value
	}
	return input
}

func mergeSettingsPatch(current AppSettings, patch map[string]any) (AppSettings, error) {
	data, err := json.Marshal(current)
	if err != nil {
		return AppSettings{}, err
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return AppSettings{}, err
	}
	if password, ok := patch["obsPassword"].(string); ok && password == "***" {
		delete(patch, "obsPassword")
	}
	mergeObject(values, patch)
	data, err = json.Marshal(values)
	if err != nil {
		return AppSettings{}, err
	}
	var updated AppSettings
	if err := json.Unmarshal(data, &updated); err != nil {
		return AppSettings{}, fmt.Errorf("设置格式无效: %w", err)
	}
	return mergeAppSettings(updated, current.Version, current.AssetsRoot), nil
}

func mergeObject(destination, source map[string]any) {
	for key, value := range source {
		if sourceChild, ok := value.(map[string]any); ok {
			if destinationChild, ok := destination[key].(map[string]any); ok {
				mergeObject(destinationChild, sourceChild)
				continue
			}
		}
		destination[key] = value
	}
}

func featureEntitlement(id FeatureID) string {
	switch id {
	case FeatureGreenWindow, FeatureVirtualCamera, FeatureImpactGift, FeatureGiftScreen:
		return "overlay"
	default:
		return "slot"
	}
}

func featureValue[T string | bool | int | float64](config FeatureConfig, key string, fallback T) T {
	value, ok := config.Values[key]
	if !ok {
		return fallback
	}
	if result, ok := value.(T); ok {
		return result
	}
	if number, ok := value.(float64); ok {
		if _, isInt := any(fallback).(int); isInt {
			return any(int(number)).(T)
		}
	}
	return fallback
}

func splitFeatureList(value string) []string {
	parts := strings.FieldsFunc(value, func(char rune) bool { return char == ',' || char == '，' })
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			items = append(items, item)
		}
	}
	return items
}
