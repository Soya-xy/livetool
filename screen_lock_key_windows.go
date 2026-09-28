//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	vkSpaceKey = 0x20
	wmQuit     = 0x0012
)

var (
	procGetCurrentThreadID = evtKernel32.NewProc("GetCurrentThreadId")
	procPostThreadMessage  = evtUser32.NewProc("PostThreadMessageW")
	screenLockCallback     = syscall.NewCallback(screenLockHookProc)
)

// screenLockHookProc 是 WH_KEYBOARD_LL 回调：只观察空格，不拦截任何按键。
func screenLockHookProc(code int32, wparam uintptr, lparam unsafe.Pointer) uintptr {
	if code >= 0 && lparam != nil {
		screenLockKeyMu.Lock()
		state := screenLockKeyObserver
		screenLockKeyMu.Unlock()
		if state != nil {
			switch wparam {
			case wmKeyDown, wmKeyUp, wmSysKeyDown, wmSysKeyUp:
				kb := (*kbdllHookStruct)(lparam)
				if uint16(kb.vkCode) == vkSpaceKey {
					state.handleKey(wparam == wmKeyDown || wparam == wmSysKeyDown, kb.flags&llkhfInjected != 0)
				}
			}
		}
	}
	// 一律放行：锁链不独占空格，其它窗口照常收到按键。
	result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wparam, uintptr(lparam))
	return result
}

// installScreenLockHook 在专属线程上安装键盘钩子并等待安装结果。
func installScreenLockHook(state *screenLockKeyState) error {
	ready := make(chan error, 1)
	go runScreenLockHookLoop(state, ready)
	return <-ready
}

// runScreenLockHookLoop 在锁定线程上跑钩子与消息循环——WH_KEYBOARD_LL 要求如此。
func runScreenLockHookLoop(state *screenLockKeyState, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	threadID, _, _ := procGetCurrentThreadID.Call()
	state.mu.Lock()
	state.threadID = uint32(threadID)
	state.mu.Unlock()

	hook, _, err := procSetWindowsHookEx.Call(whKeyboardLL, screenLockCallback, 0, 0)
	if hook == 0 {
		ready <- fmt.Errorf("安装键盘钩子失败：%v", err)
		return
	}
	state.mu.Lock()
	state.hook = hook
	state.mu.Unlock()
	ready <- nil

	var message [48]byte
	for !state.isStopped() {
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&message[0])), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
	}
}

// uninstallScreenLockHook 解除钩子并唤醒消息循环，让钩子线程正常退出。
func uninstallScreenLockHook(state *screenLockKeyState) {
	state.mu.Lock()
	hook, threadID := state.hook, state.threadID
	state.hook = 0
	state.mu.Unlock()
	if hook != 0 {
		procUnhookWindowsHook.Call(hook)
	}
	if threadID != 0 {
		procPostThreadMessage.Call(uintptr(threadID), wmQuit, 0, 0)
	}
}
