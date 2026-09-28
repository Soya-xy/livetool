//go:build windows

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	evtUser32   = syscall.NewLazyDLL("user32.dll")
	evtKernel32 = syscall.NewLazyDLL("kernel32.dll")
	evtPowrProf = syscall.NewLazyDLL("powrprof.dll")
	evtShell32  = syscall.NewLazyDLL("shell32.dll")

	procLockWorkStation   = evtUser32.NewProc("LockWorkStation")
	procGetForegroundWnd  = evtUser32.NewProc("GetForegroundWindow")
	procShowWindowMin     = evtUser32.NewProc("ShowWindow")
	procKeybdEvent        = evtUser32.NewProc("keybd_event")
	procSetWindowsHookEx  = evtUser32.NewProc("SetWindowsHookExW")
	procCallNextHookEx    = evtUser32.NewProc("CallNextHookEx")
	procUnhookWindowsHook = evtUser32.NewProc("UnhookWindowsHookEx")
	procGetMessage        = evtUser32.NewProc("GetMessageW")
	procSetSuspendState   = evtPowrProf.NewProc("SetSuspendState")
	procShellExecuteW     = evtShell32.NewProc("ShellExecuteW")
)

const (
	swMinimize    = 6
	whKeyboardLL  = 13
	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101
	wmSysKeyDown  = 0x0104
	wmSysKeyUp    = 0x0105
	llkhfInjected = 0x00000010
	vkAlt         = 0x12
	vkF4          = 0x73
	keyEventKeyUp = 0x0002
)

// platformRunBuiltinEvent 在 Windows 上执行内置事件。
// timeout 为 0 表示该事件不使用「持续时间」。
func platformRunBuiltinEvent(eventID string, action Action, timeout int) error {
	switch eventID {
	case RuleEventKillProcessName:
		return killProcessByName(action.KillProcessName)
	case RuleEventRunExe:
		return runExecutable(action.RunExePath)
	case RuleEventShutdown:
		return runShutdownTool("/s", "/t", "0", "/f")
	case RuleEventRestart:
		return runShutdownTool("/r", "/t", "0", "/f")
	case RuleEventLogoff:
		return runShutdownTool("/l", "/f")
	case RuleEventSleep:
		return suspendComputer()
	case RuleEventViewLock:
		return lockWorkstation()
	case RuleEventMinimized:
		return minimizeForegroundWindow()
	case RuleEventCFQuitRoom:
		return quitCrossFireRoom()
	case RuleEventLockAWSD, RuleEventLockWSD, RuleEventLockASD, RuleEventLockAWD, RuleEventLockAWS:
		blocked, autoPress, err := lockedKeysFor(eventID)
		if err != nil {
			return err
		}
		return holdKeyLock(blocked, autoPress, timeout)
	default:
		return errors.New("不支持的内置事件：" + eventID)
	}
}

// runShutdownTool 通过系统自带的 shutdown.exe 完成关机 / 重启 / 注销。
// 走 shutdown.exe 而不是 ExitWindowsEx，是为了避免自行申请 SeShutdownPrivilege
// 失败时静默不生效——shutdown.exe 自己会处理权限提升。
func runShutdownTool(args ...string) error {
	command := exec.Command("shutdown.exe", args...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Run(); err != nil {
		return fmt.Errorf("调用系统关机工具失败：%w", err)
	}
	return nil
}

// suspendComputer 让系统进入睡眠。
func suspendComputer() error {
	result, _, err := procSetSuspendState.Call(0, 0, 0)
	if result == 0 {
		return fmt.Errorf("进入睡眠失败：%v", err)
	}
	return nil
}

// lockWorkstation 锁定当前会话。
func lockWorkstation() error {
	result, _, err := procLockWorkStation.Call()
	if result == 0 {
		return fmt.Errorf("锁屏失败：%v", err)
	}
	return nil
}

// minimizeForegroundWindow 最小化前台窗口。
func minimizeForegroundWindow() error {
	hwnd, _, _ := procGetForegroundWnd.Call()
	if hwnd == 0 {
		return errors.New("找不到前台窗口")
	}
	procShowWindowMin.Call(hwnd, swMinimize)
	return nil
}

// runExecutable 启动用户配置的应用程序。
func runExecutable(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("应用程序路径为空")
	}
	target, _ := syscall.UTF16PtrFromString(path)
	verb, _ := syscall.UTF16PtrFromString("open")
	result, _, _ := procShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, 1)
	// ShellExecuteW 返回值 <= 32 表示失败。
	if result <= 32 {
		return fmt.Errorf("启动程序失败（ShellExecute 返回 %d）：%s", result, path)
	}
	return nil
}

// quitCrossFireRoom 向穿越火线前台窗口发送 Alt+F4。
// 原版的 handlerRule 在 cf_quit_room 分支使用 SendInput；这里用等价的 keybd_event。
func quitCrossFireRoom() error {
	hwnd, _, _ := procGetForegroundWnd.Call()
	if hwnd == 0 {
		return errors.New("找不到前台窗口")
	}
	pressAltF4()
	return nil
}

func pressAltF4() {
	procKeybdEvent.Call(vkAlt, 0, 0, 0)
	procKeybdEvent.Call(vkF4, 0, 0, 0)
	procKeybdEvent.Call(vkF4, 0, keyEventKeyUp, 0)
	procKeybdEvent.Call(vkAlt, 0, keyEventKeyUp, 0)
}

// ── 结束指定进程 ──

const (
	th32csSnapProcess  = 0x00000002
	processTerminate   = 0x0001
	invalidHandleValue = ^uintptr(0)
	maxPath            = 260
)

type processEntry32 struct {
	size            uint32
	cntUsage        uint32
	processID       uint32
	defaultHeapID   uintptr
	moduleID        uint32
	cntThreads      uint32
	parentProcessID uint32
	priClassBase    int32
	flags           uint32
	exeFile         [maxPath]uint16
}

// killProcessByName 按可执行文件名结束进程，对应原版 myapp/core/utils/process.ProcessKillByName。
func killProcessByName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("进程名为空")
	}
	procCreateSnapshot := evtKernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First := evtKernel32.NewProc("Process32FirstW")
	procProcess32Next := evtKernel32.NewProc("Process32NextW")
	procOpenProcess := evtKernel32.NewProc("OpenProcess")
	procTerminateProcess := evtKernel32.NewProc("TerminateProcess")
	procCloseHandle := evtKernel32.NewProc("CloseHandle")

	snapshot, _, _ := procCreateSnapshot.Call(th32csSnapProcess, 0)
	if snapshot == 0 || snapshot == invalidHandleValue {
		return errors.New("无法枚举进程列表")
	}
	defer procCloseHandle.Call(snapshot)

	target := strings.ToLower(name)
	entry := processEntry32{size: uint32(unsafe.Sizeof(processEntry32{}))}
	result, _, _ := procProcess32First.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	killed := 0
	for result != 0 {
		exe := strings.ToLower(syscall.UTF16ToString(entry.exeFile[:]))
		if exe == target {
			handle, _, _ := procOpenProcess.Call(processTerminate, 0, uintptr(entry.processID))
			if handle != 0 {
				if ok, _, _ := procTerminateProcess.Call(handle, 0); ok != 0 {
					killed++
				}
				procCloseHandle.Call(handle)
			}
		}
		entry.size = uint32(unsafe.Sizeof(processEntry32{}))
		result, _, _ = procProcess32Next.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	}
	if killed == 0 {
		return fmt.Errorf("没有找到可结束的进程：%s", name)
	}
	return nil
}

// ── 锁定类事件：低级键盘钩子 ──

type keyLockState struct {
	blocked   map[uint16]bool
	autoPress uint16
	hook      uintptr
	// injected 标记自己注入的按键，避免被自己的钩子吞掉。
	injected uint16
	mu       sync.Mutex
}

var (
	activeKeyLock   *keyLockState
	activeKeyLockMu sync.Mutex
	keyLockCallback = syscall.NewCallback(keyLockProc)
)

// kbdllHookStruct 对应 Win32 的 KBDLLHOOKSTRUCT。
type kbdllHookStruct struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

// keyLockProc 是 WH_KEYBOARD_LL 的回调：屏蔽被锁定的按键，放行自己注入的按键。
// lparam 直接声明为 unsafe.Pointer（runtime 的 callback 编译器接受该类型），
// 因此不需要 uintptr → unsafe.Pointer 的反向转换，go vet 也不会报误用。
func keyLockProc(code int32, wparam uintptr, lparam unsafe.Pointer) uintptr {
	if code >= 0 && lparam != nil {
		activeKeyLockMu.Lock()
		state := activeKeyLock
		activeKeyLockMu.Unlock()
		if state != nil {
			switch wparam {
			case wmKeyDown, wmKeyUp, wmSysKeyDown, wmSysKeyUp:
				kb := (*kbdllHookStruct)(lparam)
				vk := uint16(kb.vkCode)
				injected := kb.flags&llkhfInjected != 0
				state.mu.Lock()
				own := state.injected == vk
				state.mu.Unlock()
				if !injected && !own && state.blocked[vk] {
					return 1 // 吞掉该按键
				}
			}
		}
	}
	result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wparam, uintptr(lparam))
	return result
}

// holdKeyLock 安装键盘钩子并在 timeout 毫秒后解除。
// autoPress 非 0 时，锁定期间持续按住该键。
func holdKeyLock(blocked []uint16, autoPress uint16, timeout int) error {
	activeKeyLockMu.Lock()
	if activeKeyLock != nil {
		previous := activeKeyLock
		activeKeyLock = nil
		activeKeyLockMu.Unlock()
		releaseKeyLock(previous)
		activeKeyLockMu.Lock()
	}
	state := &keyLockState{blocked: make(map[uint16]bool, len(blocked)), autoPress: autoPress}
	for _, key := range blocked {
		state.blocked[key] = true
	}
	activeKeyLock = state
	activeKeyLockMu.Unlock()

	ready := make(chan error, 1)
	go runKeyLockLoop(state, ready)
	if err := <-ready; err != nil {
		activeKeyLockMu.Lock()
		if activeKeyLock == state {
			activeKeyLock = nil
		}
		activeKeyLockMu.Unlock()
		return err
	}

	if autoPress != 0 {
		state.mu.Lock()
		state.injected = autoPress
		state.mu.Unlock()
		procKeybdEvent.Call(uintptr(autoPress), 0, 0, 0)
	}

	time.Sleep(sleepDuration(timeout))

	activeKeyLockMu.Lock()
	if activeKeyLock == state {
		activeKeyLock = nil
	}
	activeKeyLockMu.Unlock()
	releaseKeyLock(state)
	return nil
}

// runKeyLockLoop 在锁定线程上跑钩子与消息循环——WH_KEYBOARD_LL 要求如此。
func runKeyLockLoop(state *keyLockState, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hook, _, err := procSetWindowsHookEx.Call(whKeyboardLL, keyLockCallback, 0, 0)
	if hook == 0 {
		ready <- fmt.Errorf("安装键盘钩子失败：%v", err)
		return
	}
	state.mu.Lock()
	state.hook = hook
	state.mu.Unlock()
	ready <- nil

	var msg [48]byte
	for {
		activeKeyLockMu.Lock()
		current := activeKeyLock
		activeKeyLockMu.Unlock()
		if current != state {
			return
		}
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
	}
}

// releaseKeyLock 解除钩子并松开自动按住的按键。
func releaseKeyLock(state *keyLockState) {
	if state.autoPress != 0 {
		procKeybdEvent.Call(uintptr(state.autoPress), 0, keyEventKeyUp, 0)
	}
	state.mu.Lock()
	hook := state.hook
	state.hook = 0
	state.mu.Unlock()
	if hook != 0 {
		procUnhookWindowsHook.Call(hook)
	}
}
