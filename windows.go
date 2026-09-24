package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func (s *AppService) OverlayOpen(kind string) ([]OverlayWindowStatus, error) {
	if err := s.requireEntitlement(overlayEntitlement(kind)); err != nil {
		return nil, err
	}
	window, err := s.ensureOverlay(kind)
	if err != nil {
		return nil, err
	}
	window.Show()
	if err := s.waitOverlayReady(kind); err != nil {
		return nil, err
	}
	s.settingsMu.Lock()
	settings := s.overlaySettingsLocked(kind)
	settings.Visible = true
	s.setOverlaySettingsLocked(kind, settings)
	s.settingsMu.Unlock()
	s.emitOverlayStatus(kind)
	return s.OverlayStatus(), nil
}

// OverlayReady is called after the overlay has subscribed to Wails events.
// It replays cached widget state in case the native window loaded after it was sent.
func (s *AppService) OverlayReady(kind string) error {
	if kind != "green" && kind != "slot" {
		return errors.New("窗口类型无效")
	}
	s.mu.Lock()
	ready := s.overlayReady[kind]
	if ready != nil && !s.overlayReadySet[kind] {
		close(ready)
		s.overlayReadySet[kind] = true
	}
	widgets := make([]OverlayWidgetPayload, 0, len(s.slotWidgets))
	if kind == "slot" {
		for _, widget := range s.slotWidgets {
			widgets = append(widgets, cloneWidget(widget))
		}
	}
	s.mu.Unlock()
	if kind == "slot" {
		settings := s.overlaySettings(kind)
		s.emitOverlayMessage("slot", "background-mode", map[string]any{"transparent": settings.BackgroundTransparent})
		for _, widget := range widgets {
			s.emitOverlayMessage("slot", "component-widget", widget)
		}
	}
	return nil
}

func (s *AppService) waitOverlayReady(kind string) error {
	s.mu.Lock()
	ready, wasReady := s.overlayReady[kind], s.overlayReadySet[kind]
	s.mu.Unlock()
	if ready == nil || wasReady {
		return nil
	}
	select {
	case <-ready:
		return nil
	case <-time.After(12 * time.Second):
		return errors.New("Overlay 窗口初始化超时，请重试")
	}
}

func (s *AppService) OverlayClose(kind string) ([]OverlayWindowStatus, error) {
	window := s.overlayWindow(kind)
	if window != nil {
		window.Hide()
	}
	s.settingsMu.Lock()
	settings := s.overlaySettingsLocked(kind)
	settings.Visible = false
	s.setOverlaySettingsLocked(kind, settings)
	s.settingsMu.Unlock()
	if err := s.persistSettings(); err != nil {
		return nil, err
	}
	s.emitOverlayStatus(kind)
	return s.OverlayStatus(), nil
}

func (s *AppService) OverlayStatus() []OverlayWindowStatus {
	return []OverlayWindowStatus{s.overlayStatus("green"), s.overlayStatus("slot")}
}

func (s *AppService) OverlaySetMode(kind, mode string) (OverlayWindowStatus, error) {
	if err := s.requireEntitlement(overlayEntitlement(kind)); err != nil {
		return OverlayWindowStatus{}, err
	}
	window, err := s.ensureOverlay(kind)
	if err != nil {
		return OverlayWindowStatus{}, err
	}
	size := windowSize(mode, s.overlaySettings(kind))
	s.mu.Lock()
	s.overlayModes[kind] = mode
	s.mu.Unlock()
	if mode == "fullscreen" {
		window.Fullscreen()
	} else {
		window.UnFullscreen()
		window.SetSize(size.Width, size.Height)
		window.Center()
		s.settingsMu.Lock()
		settings := s.overlaySettingsLocked(kind)
		settings.Width, settings.Height, settings.Visible = size.Width, size.Height, true
		s.setOverlaySettingsLocked(kind, settings)
		s.settingsMu.Unlock()
	}
	if mode == "fullscreen" {
		s.settingsMu.Lock()
		settings := s.overlaySettingsLocked(kind)
		settings.Visible = true
		s.setOverlaySettingsLocked(kind, settings)
		s.settingsMu.Unlock()
	}
	window.Show()
	if err := s.persistSettings(); err != nil {
		return OverlayWindowStatus{}, err
	}
	s.emitOverlayStatus(kind)
	return s.overlayStatus(kind), nil
}

func (s *AppService) OverlayToggleOpacity(kind string) (OverlayWindowStatus, error) {
	if err := s.requireEntitlement(overlayEntitlement(kind)); err != nil {
		return OverlayWindowStatus{}, err
	}
	if _, err := s.ensureOverlay(kind); err != nil {
		return OverlayWindowStatus{}, err
	}
	s.settingsMu.Lock()
	settings := s.overlaySettingsLocked(kind)
	if kind == "green" {
		if settings.Opacity <= 0.3 {
			settings.Opacity = 1
		} else {
			settings.Opacity = 0.25
		}
	} else {
		settings.BackgroundTransparent = !settings.BackgroundTransparent
	}
	s.setOverlaySettingsLocked(kind, settings)
	s.settingsMu.Unlock()
	if kind == "slot" {
		s.emitOverlayMessage(kind, "background-mode", map[string]any{"transparent": settings.BackgroundTransparent})
		if err := s.persistSettings(); err != nil {
			return OverlayWindowStatus{}, err
		}
	} else {
		s.emitOverlayMessage(kind, "window-opacity", map[string]any{"opacity": settings.Opacity})
	}
	s.emitOverlayStatus(kind)
	return s.overlayStatus(kind), nil
}

func (s *AppService) OverlayUpdateSettings(kind string, patch map[string]any) (OverlaySettings, error) {
	if kind != "green" && kind != "slot" {
		return OverlaySettings{}, errors.New("窗口类型无效")
	}
	if err := s.requireEntitlement(overlayEntitlement(kind)); err != nil {
		return OverlaySettings{}, err
	}
	s.settingsMu.Lock()
	settings := s.overlaySettingsLocked(kind)
	updated, err := mergeOverlaySettings(settings, patch)
	if err == nil {
		s.setOverlaySettingsLocked(kind, updated)
	}
	s.settingsMu.Unlock()
	if err != nil {
		return OverlaySettings{}, err
	}
	if err := s.applyOverlaySettings(kind, updated); err != nil {
		return OverlaySettings{}, err
	}
	if err := s.persistSettings(); err != nil {
		return OverlaySettings{}, err
	}
	s.emitOverlayStatus(kind)
	return updated, nil
}

func (s *AppService) OverlayPlayVideo(payload OverlayVideoPayload) error {
	if err := s.requireEntitlement("overlay"); err != nil {
		return err
	}
	path, err := s.resolveAssetPath(payload.Path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("找不到视频素材：%s", filepath.Base(payload.Path))
	}
	mediaPath, err := s.mediaURL(path)
	if err != nil {
		return err
	}
	if _, err := s.OverlayOpen("green"); err != nil {
		return err
	}
	payload.Path = mediaPath
	s.emitOverlayMessage("green", "play-video", payload)
	return nil
}

func (s *AppService) OverlayDrop(payload map[string]any) error {
	if err := s.requireEntitlement("overlay"); err != nil {
		return err
	}
	imagePath, _ := payload["image"].(string)
	resolved, err := s.resolveAssetPath(imagePath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(resolved); err != nil {
		return fmt.Errorf("找不到图片素材：%s", filepath.Base(imagePath))
	}
	mediaPath, err := s.mediaURL(resolved)
	if err != nil {
		return err
	}
	if _, err := s.OverlayOpen("green"); err != nil {
		return err
	}
	payload["image"] = mediaPath
	s.emitOverlayMessage("green", "drop", payload)
	return nil
}

func (s *AppService) OverlaySlot(payload map[string]any) error {
	if err := s.requireEntitlement("slot"); err != nil {
		return err
	}
	for key := range map[string]bool{"audioPath": true} {
		if path, ok := payload[key].(string); ok && path != "" {
			resolved, err := s.resolveAssetPath(path)
			if err != nil {
				return err
			}
			if info, err := os.Stat(resolved); err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("找不到音频素材：%s", filepath.Base(path))
			}
			mediaPath, err := s.mediaURL(resolved)
			if err != nil {
				return err
			}
			payload[key] = mediaPath
		}
	}
	var imagePaths []string
	switch images := payload["images"].(type) {
	case []string:
		imagePaths = images
	case []any:
		for _, image := range images {
			if path, ok := image.(string); ok {
				imagePaths = append(imagePaths, path)
			}
		}
	}
	if len(imagePaths) > 0 {
		resolvedImages := make([]string, 0, len(imagePaths))
		for _, path := range imagePaths {
			resolved, err := s.resolveAssetPath(path)
			if err != nil {
				return err
			}
			if info, err := os.Stat(resolved); err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("找不到水果机图片：%s", filepath.Base(path))
			}
			mediaPath, err := s.mediaURL(resolved)
			if err != nil {
				return err
			}
			resolvedImages = append(resolvedImages, mediaPath)
		}
		payload["images"] = resolvedImages
	}
	if _, err := s.OverlayOpen("slot"); err != nil {
		return err
	}
	s.emitOverlayMessage("slot", "slot-start", payload)
	return nil
}

func (s *AppService) OverlayWidget(payload OverlayWidgetPayload) error {
	if err := s.requireEntitlement(featureEntitlement(payload.FeatureID)); err != nil {
		return err
	}
	if payload.Values == nil {
		payload.Values = map[string]any{}
	}
	for _, key := range []string{"imagePath", "audioPath"} {
		if value, ok := payload.Values[key].(string); ok && value != "" {
			resolved, err := s.resolveAssetPath(value)
			if err != nil {
				return err
			}
			if info, err := os.Stat(resolved); err != nil || !info.Mode().IsRegular() {
				kind := "图片"
				if key == "audioPath" {
					kind = "音频"
				}
				return fmt.Errorf("找不到%s素材：%s", kind, filepath.Base(value))
			}
			mediaPath, err := s.mediaURL(resolved)
			if err != nil {
				return err
			}
			payload.Values[key] = mediaPath
		}
	}
	s.mu.Lock()
	if payload.Kind != "speech" {
		s.slotWidgets[payload.FeatureID] = cloneWidget(payload)
	}
	newWindow := s.slotWindow == nil
	s.mu.Unlock()
	if _, err := s.OverlayOpen("slot"); err != nil {
		return err
	}
	if newWindow && payload.Kind != "speech" {
		return nil
	}
	s.emitOverlayMessage("slot", "component-widget", payload)
	return nil
}

func (s *AppService) OverlayRemoveWidget(id FeatureID) {
	s.mu.Lock()
	delete(s.slotWidgets, id)
	s.mu.Unlock()
	s.emitOverlayMessage("slot", "component-remove", map[string]any{"featureId": id})
}

func (s *AppService) AudioPlay(payload map[string]any) error {
	if err := s.requireEntitlement("slot"); err != nil {
		return err
	}
	path, _ := payload["path"].(string)
	resolved, err := s.resolveAssetPath(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(resolved); err != nil {
		return fmt.Errorf("找不到音频素材：%s", filepath.Base(path))
	}
	mediaPath, err := s.mediaURL(resolved)
	if err != nil {
		return err
	}
	s.ensureAudio()
	if err := s.waitAudioReady(); err != nil {
		return err
	}
	payload["path"] = mediaPath
	s.emit("audio:message", map[string]any{"type": "play-audio", "payload": payload})
	return nil
}
func (s *AppService) AudioStop() error {
	s.ensureAudio()
	if err := s.waitAudioReady(); err != nil {
		return err
	}
	s.emit("audio:message", map[string]any{"type": "stop-audio", "payload": map[string]any{}})
	return nil
}

func (s *AppService) AudioReady() {
	s.mu.Lock()
	if s.audioReady != nil && !s.audioReadySet {
		close(s.audioReady)
		s.audioReadySet = true
	}
	s.mu.Unlock()
}

func (s *AppService) waitAudioReady() error {
	s.mu.Lock()
	ready, wasReady := s.audioReady, s.audioReadySet
	s.mu.Unlock()
	if ready == nil || wasReady {
		return nil
	}
	select {
	case <-ready:
		return nil
	case <-time.After(12 * time.Second):
		return errors.New("音频窗口初始化超时，请重试")
	}
}

func (s *AppService) ensureOverlay(kind string) (*application.WebviewWindow, error) {
	if kind != "green" && kind != "slot" {
		return nil, errors.New("窗口类型无效")
	}
	s.mu.Lock()
	if window := s.overlayWindowLocked(kind); window != nil {
		s.mu.Unlock()
		return window, nil
	}
	if creating := s.overlayCreating[kind]; creating != nil {
		s.mu.Unlock()
		<-creating
		return s.ensureOverlay(kind)
	}
	creating := make(chan struct{})
	s.overlayCreating[kind] = creating
	s.overlayReady[kind] = make(chan struct{})
	s.overlayReadySet[kind] = false
	s.mu.Unlock()
	settings := s.overlaySettings(kind)
	name, title, route := "overlay-green", "阿比整蛊 - 绿幕窗口 【禁止最小化】（按Tab键可以管理视频列表）", "/#/overlay-green"
	if kind == "slot" {
		name, title, route = "overlay-slot", "阿比整蛊 - 组件窗口 【禁止最小化】 快捷键切换透明度 Ctrl + F1", "/#/overlay-slot"
	}
	backgroundType := application.BackgroundTypeSolid
	var backgroundColour application.RGBA
	if kind == "slot" {
		// Keep the native caption while allowing transparent WebView2 pixels to
		// reveal the window underneath. The slot page paints its opaque panel
		// itself when BackgroundTransparent is false.
		backgroundType = application.BackgroundTypeTransparent
		backgroundColour = application.NewRGBA(0, 0, 0, 0)
	} else {
		backgroundColour = wailsColour(overlayBackground(kind, settings))
	}
	window := s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: name, Title: title, URL: route, Width: settings.Width, Height: settings.Height,
		MinWidth: 320, MinHeight: 240, Hidden: true, AlwaysOnTop: settings.AlwaysOnTop, Frameless: false,
		BackgroundType: backgroundType, BackgroundColour: backgroundColour,
		DisableResize: false, MinimiseButtonState: application.ButtonDisabled,
		MaximiseButtonState: application.ButtonDisabled, UseApplicationMenu: false,
	})
	s.mu.Lock()
	if kind == "green" {
		s.greenWindow = window
	} else {
		s.slotWindow = window
	}
	delete(s.overlayCreating, kind)
	close(creating)
	s.mu.Unlock()
	window.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		s.mu.Lock()
		if kind == "green" && s.greenWindow == window {
			s.greenWindow = nil
		} else if kind == "slot" && s.slotWindow == window {
			s.slotWindow = nil
		}
		s.overlayReady[kind] = nil
		s.overlayReadySet[kind] = false
		s.mu.Unlock()
		s.settingsMu.Lock()
		settings := s.overlaySettingsLocked(kind)
		settings.Visible = false
		s.setOverlaySettingsLocked(kind, settings)
		s.settingsMu.Unlock()
		if err := s.persistSettings(); err != nil {
			s.log("warn", "overlay", "窗口关闭后保存设置失败", err.Error())
		}
		s.emit("overlay:message", OverlayMessage{Target: kind, Type: "window-closed", Payload: map[string]any{}})
		s.emitOverlayStatus(kind)
	})
	return window, nil
}

func (s *AppService) ensureAudio() *application.WebviewWindow {
	s.mu.Lock()
	if s.audioWindow != nil {
		window := s.audioWindow
		s.mu.Unlock()
		return window
	}
	if creating := s.audioCreating; creating != nil {
		s.mu.Unlock()
		<-creating
		return s.ensureAudio()
	}
	creating := make(chan struct{})
	s.audioCreating = creating
	s.audioReady = make(chan struct{})
	s.audioReadySet = false
	s.mu.Unlock()
	window := s.app.Window.NewWithOptions(application.WebviewWindowOptions{Name: "audio-overlay", Title: "Audio", URL: "/#/overlay-audio", Width: 2, Height: 2, Hidden: true})
	s.mu.Lock()
	s.audioWindow = window
	s.audioCreating = nil
	close(creating)
	s.mu.Unlock()
	window.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		s.mu.Lock()
		if s.audioWindow == window {
			s.audioWindow = nil
			s.audioReady = nil
			s.audioReadySet = false
		}
		s.mu.Unlock()
	})
	return window
}

func (s *AppService) overlayWindow(kind string) *application.WebviewWindow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overlayWindowLocked(kind)
}
func (s *AppService) overlayWindowLocked(kind string) *application.WebviewWindow {
	if kind == "green" {
		return s.greenWindow
	}
	if kind == "slot" {
		return s.slotWindow
	}
	return nil
}

func (s *AppService) overlayStatus(kind string) OverlayWindowStatus {
	settings := s.overlaySettings(kind)
	window := s.overlayWindow(kind)
	width, height := settings.Width, settings.Height
	visible, fullscreen := false, false
	if window != nil {
		width, height = window.Size()
		visible, fullscreen = window.IsVisible(), window.IsFullscreen()
	}
	role, title := "ylm", "阿比直播工具 · 绿幕窗口"
	s.mu.Lock()
	mode := s.overlayModes[kind]
	s.mu.Unlock()
	if mode == "" {
		mode = "green"
	}
	transparent := false
	if kind == "slot" {
		role, title, transparent = "yapp", "阿比直播工具 · 组件窗口", settings.BackgroundTransparent
	}
	return OverlayWindowStatus{Type: kind, Role: role, Title: title, Visible: visible, Width: width, Height: height, Mode: mode, Fullscreen: fullscreen, Opacity: settings.Opacity, BackgroundTransparent: transparent}
}
func (s *AppService) emitOverlayStatus(kind string) { s.emit("overlay:status", s.overlayStatus(kind)) }
func (s *AppService) emitOverlayMessage(target, kind string, payload any) {
	s.emit("overlay:message", OverlayMessage{Target: target, Type: kind, Payload: payload})
}
func (s *AppService) overlaySettings(kind string) OverlaySettings {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.overlaySettingsLocked(kind)
}
func (s *AppService) overlaySettingsLocked(kind string) OverlaySettings {
	if kind == "green" {
		return s.settings.OverlayGreen
	}
	return s.settings.OverlaySlot
}
func (s *AppService) setOverlaySettingsLocked(kind string, value OverlaySettings) {
	if kind == "green" {
		s.settings.OverlayGreen = value
	} else {
		s.settings.OverlaySlot = value
	}
}
func (s *AppService) applyOverlaySettings(kind string, settings OverlaySettings) error {
	window := s.overlayWindow(kind)
	if window == nil {
		return nil
	}
	window.SetAlwaysOnTop(settings.AlwaysOnTop)
	if kind == "green" {
		window.SetBackgroundColour(wailsColour(overlayBackground(kind, settings)))
	}
	window.SetSize(max(320, settings.Width), max(240, settings.Height))
	if kind == "slot" {
		s.emitOverlayMessage("slot", "background-mode", map[string]any{"transparent": settings.BackgroundTransparent})
	}
	return nil
}
func (s *AppService) settingForFeature(feature FeatureID) FeatureConfig {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.settings.Features[feature]
}
func (s *AppService) saveFeatureConfig(feature FeatureID, config FeatureConfig) error {
	s.settingsMu.Lock()
	s.settings.Features[feature] = config
	s.settingsMu.Unlock()
	return s.persistSettings()
}

func overlayEntitlement(kind string) string {
	if kind == "green" {
		return "overlay"
	}
	return "slot"
}
func overlayBackground(kind string, settings OverlaySettings) string {
	if kind == "green" {
		if settings.Background == "" || strings.EqualFold(settings.Background, "transparent") {
			return "#00ff00"
		}
		return settings.Background
	}
	return "#080d18"
}
func windowSize(mode string, current OverlaySettings) OverlaySettings {
	switch mode {
	case "landscape-4-3":
		current.Width, current.Height = 800, 600
	case "portrait-9-16":
		current.Width, current.Height = 540, 960
	case "fullscreen":
	case "green":
		if current.Width < 1 {
			current.Width = 960
		}
		if current.Height < 1 {
			current.Height = 540
		}
	default:
		current.Width, current.Height = 960, 540
	}
	return current
}
func wailsColour(color string) application.RGBA {
	color = strings.TrimPrefix(color, "#")
	if len(color) != 6 {
		return application.NewRGBA(0, 0, 0, 255)
	}
	var red, green, blue uint8
	_, _ = fmt.Sscanf(color, "%02x%02x%02x", &red, &green, &blue)
	return application.NewRGBA(red, green, blue, 255)
}
func cloneWidget(payload OverlayWidgetPayload) OverlayWidgetPayload {
	copy := OverlayWidgetPayload{FeatureID: payload.FeatureID, Kind: payload.Kind, Title: payload.Title, Values: map[string]any{}, Data: map[string]any{}}
	for key, value := range payload.Values {
		copy.Values[key] = value
	}
	for key, value := range payload.Data {
		copy.Data[key] = value
	}
	return copy
}

func mergeOverlaySettings(current OverlaySettings, patch map[string]any) (OverlaySettings, error) {
	data, err := json.Marshal(current)
	if err != nil {
		return OverlaySettings{}, err
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return OverlaySettings{}, err
	}
	mergeObject(values, patch)
	data, err = json.Marshal(values)
	if err != nil {
		return OverlaySettings{}, err
	}
	var updated OverlaySettings
	if err := json.Unmarshal(data, &updated); err != nil {
		return OverlaySettings{}, err
	}
	if updated.Width < 240 || updated.Width > 7680 || updated.Height < 160 || updated.Height > 4320 {
		return OverlaySettings{}, errors.New("窗口尺寸超出有效范围")
	}
	if updated.Opacity < 0.1 || updated.Opacity > 1 {
		return OverlaySettings{}, errors.New("窗口透明度需要在 0.1 到 1 之间")
	}
	updated.LaneCount = min(max(updated.LaneCount, 1), 8)
	return updated, nil
}
