//go:build !windows

package main

import "errors"

// 非 Windows 平台没有 WH_KEYBOARD_LL，退回「组件窗获得焦点时按空格解锁」。
func installScreenLockHook(state *screenLockKeyState) error {
	return errors.New("全局键盘监听仅在 Windows 上可用")
}

func uninstallScreenLockHook(state *screenLockKeyState) {}
