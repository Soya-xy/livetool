package main

import "testing"

// 锁链的空格键观察器只统计按键，不做业务；这里覆盖重复过滤与状态机的纯逻辑。
func TestScreenLockKeyStateCountsOncePerPress(t *testing.T) {
	state := newScreenLockKeyState()
	state.handleKey(true, false)
	if got := len(state.presses); got != 1 {
		t.Fatalf("第一次按下应当记 1 次，实际 %d", got)
	}
	// 长按产生的自动重复不应重复计数。
	state.handleKey(true, false)
	state.handleKey(true, false)
	if got := len(state.presses); got != 1 {
		t.Fatalf("自动重复不应计数，实际 %d", got)
	}
	// 松开后再按算第二次。
	state.handleKey(false, false)
	state.handleKey(true, false)
	if got := len(state.presses); got != 2 {
		t.Fatalf("松开再按应当记 2 次，实际 %d", got)
	}
	// 自己注入的按键（键鼠动作）不计入。
	state.handleKey(false, false)
	state.handleKey(true, true)
	if got := len(state.presses); got != 2 {
		t.Fatalf("注入按键不应计数，实际 %d", got)
	}
}

// 没有锁链在屏时，全局空格键回调必须是安全的空操作。
func TestScreenLockKeyPressWithoutLock(t *testing.T) {
	service := &AppService{}
	if remaining := service.OverlayDecrementScreenLock(); remaining != -1 {
		t.Fatalf("没有锁链在屏时应当返回 -1，实际 %d", remaining)
	}
	service.handleScreenLockKeyPress()
	if service.screenLockKeyOn {
		t.Error("没有安装观察器时不应标记为已生效")
	}
	stopScreenLockKeyObserver()
}
