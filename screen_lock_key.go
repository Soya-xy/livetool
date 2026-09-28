package main

import "sync"

// 锁链的空格键观察器。
//
// 需求是「按空格减少次数，但不能独占空格」——其它窗口仍然要收到空格。
// Windows 的 RegisterHotKey 会把按键吞掉，所以这里改用低级键盘钩子
// （WH_KEYBOARD_LL）只统计、不拦截：钩子回调一律 CallNextHookEx，
// 因此空格照常送达前台窗口，包括本应用自己的组件窗。
//
// 钩子回调必须极快返回（阻塞会拖慢整个系统的输入），所以它只把「按下了一次」
// 投进带缓冲的通道，真正的扣次数由单独的 goroutine 处理。
//
// 非 Windows 平台不安装钩子（installScreenLockHook 直接返回错误），
// 此时返回 false，前端退回「组件窗获得焦点时按空格解锁」的旧行为。
type screenLockKeyState struct {
	presses chan struct{}
	stop    chan struct{}

	mu      sync.Mutex
	pressed bool
	stopped bool

	// 仅 Windows 使用：钩子句柄与安装钩子的线程号。
	hook     uintptr
	threadID uint32
}

func newScreenLockKeyState() *screenLockKeyState {
	return &screenLockKeyState{presses: make(chan struct{}, 8), stop: make(chan struct{})}
}

// handleKey 由钩子回调调用：只记录按下，不做任何业务逻辑。
func (s *screenLockKeyState) handleKey(down, injected bool) {
	if injected {
		// 自己注入的按键（键鼠动作、内置事件）不算用户按下的空格。
		return
	}
	s.mu.Lock()
	if down {
		if s.pressed {
			s.mu.Unlock()
			return // 长按的自动重复只算一次
		}
		s.pressed = true
		s.mu.Unlock()
		select {
		case s.presses <- struct{}{}:
		default:
			// 来不及处理就丢弃，绝不在钩子里阻塞。
		}
		return
	}
	s.pressed = false
	s.mu.Unlock()
}

func (s *screenLockKeyState) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

var (
	screenLockKeyMu       sync.Mutex
	screenLockKeyObserver *screenLockKeyState
)

// startScreenLockKeyObserver 安装全局空格键观察器；返回是否生效。
func startScreenLockKeyObserver(onPress func()) bool {
	screenLockKeyMu.Lock()
	if screenLockKeyObserver != nil {
		screenLockKeyMu.Unlock()
		return true
	}
	state := newScreenLockKeyState()
	screenLockKeyObserver = state
	screenLockKeyMu.Unlock()

	if err := installScreenLockHook(state); err != nil {
		screenLockKeyMu.Lock()
		if screenLockKeyObserver == state {
			screenLockKeyObserver = nil
		}
		screenLockKeyMu.Unlock()
		return false
	}

	go func() {
		for {
			select {
			case <-state.presses:
				onPress()
			case <-state.stop:
				return
			}
		}
	}()
	return true
}

// stopScreenLockKeyObserver 卸载观察器；重复调用无副作用。
func stopScreenLockKeyObserver() {
	screenLockKeyMu.Lock()
	state := screenLockKeyObserver
	screenLockKeyObserver = nil
	screenLockKeyMu.Unlock()
	if state == nil {
		return
	}
	state.mu.Lock()
	state.stopped = true
	state.mu.Unlock()
	uninstallScreenLockHook(state)
	close(state.stop)
}
