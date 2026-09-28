package main

import (
	"context"
	"path/filepath"
	"testing"
)

// 每次启动都要把绿幕窗口与组件窗口置为关闭：上一轮运行时被自动打开的窗口
// 不能留下 visible=true，其余窗口设置（尺寸、底板色等）必须原样保留。
func TestStartupResetsOverlayWindows(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	saved := defaultAppSettings("test", filepath.Join(dir, "assets"))
	saved.OverlayGreen.Visible = true
	saved.OverlayGreen.Width = 1280
	saved.OverlayGreen.Height = 720
	saved.OverlayGreen.Opacity = 0.25
	saved.OverlaySlot.Visible = true
	saved.OverlaySlot.BackgroundTransparent = false
	if err := store.SetSetting(ctx, "settings", saved); err != nil {
		t.Fatalf("写入设置失败: %v", err)
	}

	service, err := NewAppService(store, dir, "test")
	if err != nil {
		t.Fatalf("创建服务失败: %v", err)
	}
	defer service.stopEventWorkers()

	for _, kind := range []string{"green", "slot"} {
		if settings := service.overlaySettings(kind); settings.Visible {
			t.Errorf("%s 窗口启动后应当处于关闭状态: %+v", kind, settings)
		}
	}
	if status := service.OverlayStatus(); status[0].Visible || status[1].Visible {
		t.Errorf("窗口状态应为不可见: %+v", status)
	}

	var stored AppSettings
	if err := store.GetSetting(ctx, "settings", defaultAppSettings("test", dir), &stored); err != nil {
		t.Fatalf("读取设置失败: %v", err)
	}
	if stored.OverlayGreen.Visible || stored.OverlaySlot.Visible {
		t.Errorf("关闭状态没有写回数据库: %+v", stored)
	}
	if stored.OverlayGreen.Width != 1280 || stored.OverlayGreen.Height != 720 || stored.OverlayGreen.Opacity != 0.25 {
		t.Errorf("其余窗口设置被改动: %+v", stored.OverlayGreen)
	}
	if stored.OverlaySlot.BackgroundTransparent {
		t.Errorf("组件窗底板设置被改动: %+v", stored.OverlaySlot)
	}
}
