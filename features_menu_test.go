package main

import (
	"path/filepath"
	"testing"
	"time"
)

// 礼物菜单点击 → 一条礼物事件 → 规则链路 → 弹幕日记。
func TestFeatureMenuGiftEnqueuesGiftEvent(t *testing.T) {
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

	if result := service.FeatureMenuGift("啤酒"); !result.OK {
		t.Fatalf("礼物菜单触发失败: %s", result.Message)
	}
	if result := service.FeatureMenuGift("   "); result.OK {
		t.Error("空礼物名称应当被拒绝")
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		records, err := service.DanmakuQuery(DanmakuFilter{Limit: 5})
		if err != nil {
			t.Fatalf("查询弹幕日记失败: %v", err)
		}
		if len(records) > 0 {
			if records[0].Kind != KindGift || records[0].GiftName != "啤酒" {
				t.Fatalf("日记内容不正确: %+v", records[0])
			}
			if records[0].UserName != "礼物菜单" {
				t.Errorf("来源用户应为「礼物菜单」，实际 %q", records[0].UserName)
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("礼物菜单事件没有进入队列")
}

// 礼物图标表：连接器写入后按名称可查，未知名称返回空串。
func TestFeatureGiftIconLookup(t *testing.T) {
	service := &AppService{}
	if got := service.FeatureGiftIcon("啤酒"); got != "" {
		t.Errorf("空表应返回空串，实际 %q", got)
	}
	service.setGiftIcons(map[string]string{"啤酒": "https://example.com/beer.png", "": "https://example.com/empty.png"})
	if got := service.FeatureGiftIcon("啤酒"); got != "https://example.com/beer.png" {
		t.Errorf("图标地址不正确: %q", got)
	}
	if got := service.FeatureGiftIcon(" 啤酒 "); got != "https://example.com/beer.png" {
		t.Errorf("名称应当去掉首尾空格: %q", got)
	}
	if got := service.FeatureGiftIcon("不存在"); got != "" {
		t.Errorf("未知礼物应返回空串，实际 %q", got)
	}
}

// 组件布局保存：越界值会被钳制，未知组件被拒绝。
func TestOverlaySaveWidgetLayout(t *testing.T) {
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

	if result := service.OverlaySaveWidgetLayout("unknown-feature", WidgetLayout{X: 1, Y: 2, W: 100, H: 100}); result.OK {
		t.Error("未知组件应当被拒绝")
	}
	result := service.OverlaySaveWidgetLayout("gift-pool", WidgetLayout{X: 120, Y: 90, W: 10, H: 5})
	if !result.OK {
		t.Fatalf("保存布局失败: %s", result.Message)
	}
	saved := service.OverlayWidgetLayouts()["gift-pool"]
	if saved.W != 120 || saved.H != 90 {
		t.Errorf("过小的尺寸应被钳制到 120×90，实际 %+v", saved)
	}
	if saved.X != 120 || saved.Y != 90 {
		t.Errorf("坐标不正确: %+v", saved)
	}
}
