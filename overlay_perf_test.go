package main

import (
	"path/filepath"
	"testing"
)

// 组件窗性能显示开关：设置页写入 showPerf 后，窗口状态里能读到，且会被持久化。
func TestOverlayShowPerfRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	service, err := NewAppService(store, dir, "test")
	if err != nil {
		t.Fatalf("创建服务失败: %v", err)
	}
	defer func() {
		service.stopEventWorkers()
		store.Close()
	}()

	if status := service.OverlayStatus(); status[1].ShowPerf {
		t.Errorf("默认应当是关闭的: %+v", status[1])
	}
	updated, err := service.OverlayUpdateSettings("slot", map[string]any{"showPerf": true})
	if err != nil {
		t.Fatalf("更新设置失败: %v", err)
	}
	if !updated.ShowPerf {
		t.Errorf("返回值里应带上 showPerf: %+v", updated)
	}
	if slot := service.overlayStatus("slot"); !slot.ShowPerf {
		t.Errorf("窗口状态里应带上 showPerf: %+v", slot)
	}
	if green := service.overlayStatus("green"); green.ShowPerf {
		t.Errorf("绿幕窗口不受影响: %+v", green)
	}

	// 重新打开服务：设置应当已经落库。
	reopened, err := NewAppService(store, dir, "test")
	if err != nil {
		t.Fatalf("重开服务失败: %v", err)
	}
	defer reopened.stopEventWorkers()
	if !reopened.overlaySettings("slot").ShowPerf {
		t.Error("showPerf 没有持久化")
	}

	if _, err := service.OverlayUpdateSettings("slot", map[string]any{"showPerf": false}); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if service.overlayStatus("slot").ShowPerf {
		t.Error("关闭后状态里不应再显示 showPerf")
	}
}
