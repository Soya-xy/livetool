package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var orderedFeatures = []FeatureID{
	FeatureGreenWindow, FeatureComponentWindow, FeatureVirtualCamera, FeatureSpeedCurve, FeatureSpeedIba,
	FeatureImpactGift, FeatureFryingPan, FeatureVoiceBroadcast, FeatureDanmakuAssistant, FeatureLiveClock,
	FeatureWoodfish, FeatureSlotMachine, FeatureGiftScreen, FeatureLottery, FeatureCounter, FeatureGiftPool,
	FeatureScreenLock, FeatureMosquitoSlap,
}

func featureIDs() []FeatureID { return orderedFeatures }

func containsPlatform(platform Platform) bool {
	for _, item := range supportedPlatforms {
		if item == platform {
			return true
		}
	}
	return false
}

func cloneAuthStatus(status LicenseAuthStatus) LicenseAuthStatus {
	status.Features = append([]string(nil), status.Features...)
	return status
}

func (s *AppService) resolveAssetPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "file://") {
		return "", fmt.Errorf("请输入素材目录中的相对路径")
	}
	root, err := filepath.Abs(s.settingSnapshot().AssetsRoot)
	if err != nil {
		return "", err
	}
	candidate := input
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, filepath.FromSlash(candidate))
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("素材路径必须位于素材目录内")
	}
	return candidate, nil
}

func (s *AppService) buildExportConfig() (ExportConfig, error) {
	rules, err := s.store.ListRules(context.Background())
	if err != nil {
		return ExportConfig{}, err
	}
	assets, err := s.DiagnosticsAssets()
	if err != nil {
		return ExportConfig{}, err
	}
	settings := s.settingSnapshot()
	return ExportConfig{
		SchemaVersion: 1, AppVersion: s.version, Rules: rules, Assets: assets,
		Overlay:    OverlayExport{Green: settings.OverlayGreen, Slot: settings.OverlaySlot},
		Slot:       map[string]any{"theme": "default"},
		Connectors: ConnectorExport{Platform: settings.Platform, RoomID: settings.RoomID},
		Danmaku:    DanmakuExport{RetentionDays: settings.RetentionDays, RetentionMaxRows: settings.RetentionMaxRows, StoreRaw: settings.StoreRaw},
		Features:   settings.Features,
	}, nil
}

func formatRecords(records []DanmakuRecord, format string) (string, error) {
	if format == "json" {
		data, err := json.MarshalIndent(records, "", "  ")
		return string(data), err
	}
	if format == "csv" {
		var output strings.Builder
		writer := csv.NewWriter(&output)
		_ = writer.Write([]string{"时间", "平台", "类型", "昵称", "内容", "命中规则", "结果"})
		for _, record := range records {
			content := record.Text
			if content == "" {
				content = record.GiftName
			}
			_ = writer.Write([]string{formatDateTime(record.TS), record.Source, string(record.Kind), record.UserName, content, record.MatchedRuleName, record.ActionResult})
		}
		writer.Flush()
		return strings.TrimSuffix(output.String(), "\r\n"), writer.Error()
	}
	lines := make([]string, len(records))
	for i, record := range records {
		content := record.Text
		if content == "" {
			content = record.GiftName
		}
		lines[i] = fmt.Sprintf("[%s] [%s] %s %s %s %s", formatTime(record.TS), record.Source, record.UserName, content, record.ActionResult, record.FailReason)
	}
	return strings.Join(lines, "\n"), nil
}

func formatDateTime(timestamp int64) string {
	return timeFromMillis(timestamp).Format("2006-01-02 15:04:05")
}
func formatTime(timestamp int64) string { return timeFromMillis(timestamp).Format("15:04:05") }

func formatFeatureTemplate(template string, event LiveEvent) string {
	text, gift, count := event.Text, giftName(event), 1
	if event.Gift != nil {
		count = max(1, event.Gift.Count)
	}
	if event.Count > 0 {
		count = event.Count
	}
	for _, pair := range [][2]string{{"{user}", userName(event)}, {"{text}", text}, {"{gift}", gift}, {"{count}", fmt.Sprint(count)}} {
		template = strings.ReplaceAll(template, pair[0], pair[1])
	}
	return template
}

func timeFromMillis(value int64) time.Time { return time.UnixMilli(value) }

func waitForContext(ctx context.Context, duration time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
