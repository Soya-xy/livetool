//go:build windows && amd64

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	keyEventUp = 0x0002

	mouseMove        = 0x0001
	mouseLeftDown    = 0x0002
	mouseLeftUp      = 0x0004
	mouseRightDown   = 0x0008
	mouseRightUp     = 0x0010
	mouseMiddleDown  = 0x0020
	mouseMiddleUp    = 0x0040
	mouseWheel       = 0x0800
	mouseAbsolute    = 0x8000
	mouseVirtualDesk = 0x4000

	processQueryLimitedInformation = 0x1000
	showWindowRestore              = 9
)

var (
	inputUser32   = syscall.NewLazyDLL("user32.dll")
	inputKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSendInput                  = inputUser32.NewProc("SendInput")
	procEnumWindows                = inputUser32.NewProc("EnumWindows")
	procIsWindowVisible            = inputUser32.NewProc("IsWindowVisible")
	procGetWindowThreadProcessID   = inputUser32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW             = inputUser32.NewProc("GetWindowTextW")
	procGetClassNameW              = inputUser32.NewProc("GetClassNameW")
	procGetForegroundWindow        = inputUser32.NewProc("GetForegroundWindow")
	procSetForegroundWindow        = inputUser32.NewProc("SetForegroundWindow")
	procShowWindow                 = inputUser32.NewProc("ShowWindow")
	procClientToScreen             = inputUser32.NewProc("ClientToScreen")
	procGetSystemMetrics           = inputUser32.NewProc("GetSystemMetrics")
	procOpenProcess                = inputKernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = inputKernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle                = inputKernel32.NewProc("CloseHandle")
)

type input64 struct {
	typ     uint32
	padding uint32
	data    [32]byte
}

type keyboardInput64 struct {
	virtualKey uint16
	scanCode   uint16
	flags      uint32
	time       uint32
	extraInfo  uintptr
}

type mouseInput64 struct {
	dx        int32
	dy        int32
	data      uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type screenPoint struct{ x, y int32 }

func runSystemInput(ctx context.Context, action Action) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(action.Steps) == 0 || len(action.Steps) > 256 {
		return errors.New("键鼠步骤数量需要在 1 到 256 之间")
	}
	target, err := focusInputTarget(action.Target)
	if err != nil {
		return err
	}
	if action.Kind == "key" {
		return runKeySteps(ctx, action.Steps)
	}
	if action.Kind == "mouse" {
		return runMouseSteps(ctx, action.Steps, target)
	}
	return errors.New("键鼠动作类型无效")
}

func runKeySteps(ctx context.Context, steps []map[string]any) (result error) {
	held := map[uint16]bool{}
	defer func() {
		for key := range held {
			_ = sendKey(key, true)
		}
	}()
	for index, step := range steps {
		op := strings.ToLower(strings.TrimSpace(stringValue(step["op"])))
		if op == "wait" {
			duration, err := waitStepDuration(step)
			if err != nil {
				return fmt.Errorf("第 %d 步：%w", index+1, err)
			}
			if !waitForContext(ctx, duration) {
				return ctx.Err()
			}
			continue
		}
		key, ok := virtualKey(stringValue(step["key"]))
		if !ok {
			return fmt.Errorf("第 %d 步的按键无效：%q", index+1, stringValue(step["key"]))
		}
		switch op {
		case "down":
			if !held[key] {
				if err := sendKey(key, false); err != nil {
					return fmt.Errorf("第 %d 步按下失败：%w", index+1, err)
				}
				held[key] = true
			}
		case "up":
			if err := sendKey(key, true); err != nil {
				return fmt.Errorf("第 %d 步抬起失败：%w", index+1, err)
			}
			delete(held, key)
		case "tap":
			if err := sendKey(key, false); err != nil {
				return fmt.Errorf("第 %d 步按下失败：%w", index+1, err)
			}
			held[key] = true
			if err := sendKey(key, true); err != nil {
				return fmt.Errorf("第 %d 步抬起失败：%w", index+1, err)
			}
			delete(held, key)
		default:
			return fmt.Errorf("第 %d 步操作无效：%q", index+1, op)
		}
	}
	return nil
}

func runMouseSteps(ctx context.Context, steps []map[string]any, target uintptr) (result error) {
	held := map[uint32]bool{}
	defer func() {
		for flags := range held {
			_ = sendMouse(flags, 0, 0, 0)
		}
	}()
	for index, step := range steps {
		op := strings.ToLower(strings.TrimSpace(stringValue(step["op"])))
		if op == "wait" {
			duration, err := waitStepDuration(step)
			if err != nil {
				return fmt.Errorf("第 %d 步：%w", index+1, err)
			}
			if !waitForContext(ctx, duration) {
				return ctx.Err()
			}
			continue
		}
		if op == "scroll" {
			delta, err := integerValue(step["delta"])
			if err != nil || delta < math.MinInt32 || delta > math.MaxInt32 {
				return fmt.Errorf("第 %d 步滚轮值无效", index+1)
			}
			if err := sendMouse(mouseWheel, uint32(int32(delta)), 0, 0); err != nil {
				return fmt.Errorf("第 %d 步滚轮操作失败：%w", index+1, err)
			}
			continue
		}
		_, down, up, err := mouseButton(stringValue(step["button"]))
		if err != nil {
			return fmt.Errorf("第 %d 步：%w", index+1, err)
		}
		if op == "move" || op == "click" || op == "down" || op == "up" {
			hasX, hasY := hasNumber(step["x"]), hasNumber(step["y"])
			if hasX != hasY {
				return fmt.Errorf("第 %d 步必须同时填写 x 和 y", index+1)
			}
			if op == "move" && !hasX {
				return fmt.Errorf("第 %d 步移动操作需要 x 和 y", index+1)
			}
			if hasX {
				x, xErr := integerValue(step["x"])
				y, yErr := integerValue(step["y"])
				if xErr != nil || yErr != nil || x < math.MinInt32 || x > math.MaxInt32 || y < math.MinInt32 || y > math.MaxInt32 {
					return fmt.Errorf("第 %d 步坐标无效", index+1)
				}
				if target != 0 {
					var point screenPoint = screenPoint{int32(x), int32(y)}
					ok, _, callErr := procClientToScreen.Call(target, uintptr(unsafe.Pointer(&point)))
					if ok == 0 {
						return fmt.Errorf("第 %d 步无法将目标窗口坐标转换为屏幕坐标：%w", index+1, callErr)
					}
					x, y = int64(point.x), int64(point.y)
				}
				if err := sendMouseAbsolute(int32(x), int32(y)); err != nil {
					return fmt.Errorf("第 %d 步移动失败：%w", index+1, err)
				}
			}
		}
		switch op {
		case "move":
		case "click":
			if err := sendMouse(down, 0, 0, 0); err != nil {
				return fmt.Errorf("第 %d 步点击按下失败：%w", index+1, err)
			}
			held[up] = true
			if err := sendMouse(up, 0, 0, 0); err != nil {
				return fmt.Errorf("第 %d 步点击抬起失败：%w", index+1, err)
			}
			delete(held, up)
		case "down":
			if !held[up] {
				if err := sendMouse(down, 0, 0, 0); err != nil {
					return fmt.Errorf("第 %d 步按下失败：%w", index+1, err)
				}
				held[up] = true
			}
		case "up":
			if err := sendMouse(up, 0, 0, 0); err != nil {
				return fmt.Errorf("第 %d 步抬起失败：%w", index+1, err)
			}
			delete(held, up)
		default:
			return fmt.Errorf("第 %d 步操作无效：%q", index+1, op)
		}
	}
	return nil
}

func sendKey(key uint16, up bool) error {
	flags := uint32(0)
	if up {
		flags = keyEventUp
	}
	item := input64{typ: inputKeyboard}
	*(*keyboardInput64)(unsafe.Pointer(&item.data[0])) = keyboardInput64{virtualKey: key, flags: flags}
	return sendInput(item)
}

func sendMouse(flags, data uint32, x, y int32) error {
	item := input64{typ: inputMouse}
	*(*mouseInput64)(unsafe.Pointer(&item.data[0])) = mouseInput64{dx: x, dy: y, data: data, flags: flags}
	return sendInput(item)
}

func sendMouseAbsolute(x, y int32) error {
	originX := int32(uint32(systemMetric(76)))
	originY := int32(uint32(systemMetric(77)))
	width := int32(uint32(systemMetric(78)))
	height := int32(uint32(systemMetric(79)))
	if width <= 1 || height <= 1 || x < originX || y < originY || x >= originX+width || y >= originY+height {
		return errors.New("鼠标坐标超出桌面范围")
	}
	absX := int32(int64(x-originX) * 65535 / int64(width-1))
	absY := int32(int64(y-originY) * 65535 / int64(height-1))
	return sendMouse(mouseMove|mouseAbsolute|mouseVirtualDesk, 0, absX, absY)
}

func sendInput(item input64) error {
	count, _, callErr := procSendInput.Call(1, uintptr(unsafe.Pointer(&item)), unsafe.Sizeof(item))
	runtime.KeepAlive(item)
	if count != 1 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("SendInput 失败：%w", callErr)
		}
		return errors.New("Windows 未接受键鼠输入；请检查目标程序权限级别")
	}
	return nil
}

func focusInputTarget(target *WindowTarget) (uintptr, error) {
	if target == nil {
		return 0, nil
	}
	if strings.TrimSpace(target.Title) == "" && strings.TrimSpace(target.ClassName) == "" && strings.TrimSpace(target.ProcessName) == "" {
		return 0, errors.New("目标窗口条件为空；请填写标题、窗口类名或进程名")
	}
	var found uintptr
	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		if !windowMatches(hwnd, target) {
			return 1
		}
		found = hwnd
		return 0
	})
	_, _, callErr := procEnumWindows.Call(callback, 0)
	runtime.KeepAlive(callback)
	if found == 0 {
		if callErr != syscall.Errno(0) {
			return 0, fmt.Errorf("查找目标窗口失败：%w", callErr)
		}
		return 0, errors.New("没有找到匹配的可见窗口；未发送键鼠输入")
	}
	procShowWindow.Call(found, showWindowRestore)
	activated, _, callErr := procSetForegroundWindow.Call(found)
	if activated == 0 {
		return 0, fmt.Errorf("Windows 未允许切换到目标窗口：%w", callErr)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		active, _, _ := procGetForegroundWindow.Call()
		if active == found {
			return found, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return 0, errors.New("目标窗口未获得焦点；未发送键鼠输入")
}

func windowMatches(hwnd uintptr, target *WindowTarget) bool {
	if title := strings.TrimSpace(target.Title); title != "" && !strings.Contains(strings.ToLower(windowTitle(hwnd)), strings.ToLower(title)) {
		return false
	}
	if className := strings.TrimSpace(target.ClassName); className != "" && !strings.EqualFold(windowClass(hwnd), className) {
		return false
	}
	if processName := strings.TrimSpace(target.ProcessName); processName != "" && !strings.EqualFold(normalizeProcessName(windowProcess(hwnd)), normalizeProcessName(processName)) {
		return false
	}
	return true
}

func normalizeProcessName(value string) string {
	name := filepath.Base(strings.TrimSpace(value))
	if filepath.Ext(name) == "" {
		name += ".exe"
	}
	return strings.ToLower(name)
}

func windowTitle(hwnd uintptr) string {
	buffer := make([]uint16, 512)
	length, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return utf16ToString(buffer[:int(length)])
}

func windowClass(hwnd uintptr) string {
	buffer := make([]uint16, 256)
	length, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return utf16ToString(buffer[:int(length)])
}

func windowProcess(hwnd uintptr) string {
	var processID uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&processID)))
	process, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(processID))
	if process == 0 {
		return ""
	}
	defer procCloseHandle.Call(process)
	buffer := make([]uint16, 1024)
	length := uint32(len(buffer))
	ok, _, _ := procQueryFullProcessImageNameW.Call(process, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&length)))
	if ok == 0 || length == 0 {
		return ""
	}
	return filepath.Base(utf16ToString(buffer[:length]))
}

func utf16ToString(value []uint16) string {
	end := len(value)
	for end > 0 && value[end-1] == 0 {
		end--
	}
	return string(utf16.Decode(value[:end]))
}

func systemMetric(index uintptr) uintptr {
	value, _, _ := procGetSystemMetrics.Call(index)
	return value
}

func virtualKey(raw string) (uint16, bool) {
	key := strings.ToUpper(strings.TrimSpace(raw))
	if len(key) == 1 {
		c := key[0]
		if c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			return uint16(c), true
		}
		if value, ok := map[byte]uint16{'-': 0xBD, '=': 0xBB, '[': 0xDB, ']': 0xDD, '\\': 0xDC, ';': 0xBA, '\'': 0xDE, '`': 0xC0, ',': 0xBC, '.': 0xBE, '/': 0xBF}[c]; ok {
			return value, true
		}
	}
	keys := map[string]uint16{
		"CTRL": 0x11, "CONTROL": 0x11, "SHIFT": 0x10, "ALT": 0x12, "WIN": 0x5B, "META": 0x5B,
		"ENTER": 0x0D, "RETURN": 0x0D, "ESC": 0x1B, "ESCAPE": 0x1B, "SPACE": 0x20, "SPACEBAR": 0x20,
		"TAB": 0x09, "BACKSPACE": 0x08, "DELETE": 0x2E, "INSERT": 0x2D, "HOME": 0x24, "END": 0x23,
		"PAGEUP": 0x21, "PAGEDOWN": 0x22, "UP": 0x26, "DOWN": 0x28, "LEFT": 0x25, "RIGHT": 0x27,
		"CAPSLOCK": 0x14, "NUMLOCK": 0x90, "SCROLLLOCK": 0x91, "PAUSE": 0x13, "PRINTSCREEN": 0x2C,
		"F1": 0x70, "F2": 0x71, "F3": 0x72, "F4": 0x73, "F5": 0x74, "F6": 0x75,
		"F7": 0x76, "F8": 0x77, "F9": 0x78, "F10": 0x79, "F11": 0x7A, "F12": 0x7B,
		"F13": 0x7C, "F14": 0x7D, "F15": 0x7E, "F16": 0x7F, "F17": 0x80, "F18": 0x81,
		"F19": 0x82, "F20": 0x83, "F21": 0x84, "F22": 0x85, "F23": 0x86, "F24": 0x87,
		"NUMPAD0": 0x60, "NUMPAD1": 0x61, "NUMPAD2": 0x62, "NUMPAD3": 0x63, "NUMPAD4": 0x64,
		"NUMPAD5": 0x65, "NUMPAD6": 0x66, "NUMPAD7": 0x67, "NUMPAD8": 0x68, "NUMPAD9": 0x69,
	}
	value, ok := keys[key]
	return value, ok
}

func mouseButton(raw string) (button, down, up uint32, err error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		name = "left"
	}
	switch name {
	case "left":
		return 0, mouseLeftDown, mouseLeftUp, nil
	case "right":
		return 0, mouseRightDown, mouseRightUp, nil
	case "middle":
		return 0, mouseMiddleDown, mouseMiddleUp, nil
	default:
		return 0, 0, 0, fmt.Errorf("鼠标按键无效：%q", raw)
	}
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func hasNumber(value any) bool {
	_, err := integerValue(value)
	return err == nil
}

func integerValue(value any) (int64, error) {
	var number float64
	switch item := value.(type) {
	case float64:
		number = item
	case float32:
		number = float64(item)
	case int:
		return int64(item), nil
	case int32:
		return int64(item), nil
	case int64:
		return item, nil
	case uint:
		if uint64(item) > math.MaxInt64 {
			return 0, errors.New("integer out of range")
		}
		return int64(item), nil
	case uint32:
		return int64(item), nil
	case uint64:
		if item > math.MaxInt64 {
			return 0, errors.New("integer out of range")
		}
		return int64(item), nil
	default:
		return 0, errors.New("not a number")
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number < math.MinInt64 || number > math.MaxInt64 {
		return 0, errors.New("not an integer")
	}
	return int64(number), nil
}

func waitStepDuration(step map[string]any) (time.Duration, error) {
	value, err := integerValue(step["ms"])
	if err != nil || value < 0 || value > 60_000 {
		return 0, errors.New("等待时长需要在 0 到 60000 毫秒之间")
	}
	return time.Duration(value) * time.Millisecond, nil
}
