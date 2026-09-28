package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// 内置事件 ID —— 与《AKA直播》新版前端下拉、以及主程序 Go 侧 switch 完全一致。
// 事件 ID 与中文名的对照来自前端 i18n，Go 侧在 0xF96E00–0xF97800 的 dispatch
// switch 中逐个比对确认（lock_awsd 由 lock_aws 前缀 + 第 9 字节 d 分块比较）。
const (
	RuleEventKillProcessName = "kill_process_name"
	RuleEventRunExe          = "run_exe"
	RuleEventShutdown        = "shutdown"
	RuleEventRestart         = "restart"
	RuleEventSleep           = "sleep"
	RuleEventViewLock        = "viewlock"
	RuleEventLogoff          = "logoff"
	RuleEventMinimized       = "minimized"
	RuleEventLockAWSD        = "lock_awsd"
	RuleEventLockWSD         = "lock_wsd"
	RuleEventLockASD         = "lock_asd"
	RuleEventLockAWD         = "lock_awd"
	RuleEventLockAWS         = "lock_aws"
	RuleEventCFQuitRoom      = "cf_quit_room"
)

// defaultRuleEventTimeoutMS 对应原版 addFormHandler 里 rule_event_timeout 的默认值 "1000"。
const defaultRuleEventTimeoutMS = 1000

// maxRuleEventTimeoutMS 防止锁定类事件把键盘锁死过久。
const maxRuleEventTimeoutMS = 600000

// builtinEventDefinition 描述一个内置事件，供界面与校验共用。
type builtinEventDefinition struct {
	ID     string
	Label  string
	Timed  bool // 是否使用「持续时间(ms)」
	Needs  string
	Detail string
}

// builtinEventDefinitions 的顺序与原版下拉一致。
var builtinEventDefinitions = []builtinEventDefinition{
	{ID: RuleEventKillProcessName, Label: "结束指定进程名", Needs: "killProcessName", Detail: "按进程名结束进程"},
	{ID: RuleEventRunExe, Label: "运行一个应用程序", Needs: "runExePath", Detail: "启动指定路径的程序"},
	{ID: RuleEventShutdown, Label: "关机", Detail: "关闭计算机"},
	{ID: RuleEventRestart, Label: "重启", Detail: "重启计算机"},
	{ID: RuleEventSleep, Label: "睡眠", Detail: "让计算机进入睡眠"},
	{ID: RuleEventViewLock, Label: "锁屏", Detail: "锁定当前会话"},
	{ID: RuleEventLogoff, Label: "注销", Detail: "注销当前用户"},
	{ID: RuleEventMinimized, Label: "最小化当前窗口", Detail: "最小化前台窗口"},
	{ID: RuleEventLockAWSD, Label: "锁定AWSD(罚站)", Timed: true, Detail: "屏蔽 W/A/S/D"},
	{ID: RuleEventLockWSD, Label: "锁定WSD，自动按A(自动向左)", Timed: true, Detail: "屏蔽 W/S/D，按住 A"},
	{ID: RuleEventLockASD, Label: "锁定ASD，自动按W(自动向前)", Timed: true, Detail: "屏蔽 A/S/D，按住 W"},
	{ID: RuleEventLockAWD, Label: "锁定WAD，自动按S(自动向后)", Timed: true, Detail: "屏蔽 W/A/D，按住 S"},
	{ID: RuleEventLockAWS, Label: "锁定WAS，自动按D(自动向右)", Timed: true, Detail: "屏蔽 W/A/S，按住 D"},
	{ID: RuleEventCFQuitRoom, Label: "退出房间(穿越火线)", Detail: "向穿越火线前台窗口发送退出房间按键"},
}

func builtinEventByID(id string) (builtinEventDefinition, bool) {
	for _, definition := range builtinEventDefinitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return builtinEventDefinition{}, false
}

// builtinEventTimeout 解析「持续时间(ms)」，钳制在 1–600000。
func builtinEventTimeout(value int) int {
	if value <= 0 {
		return defaultRuleEventTimeoutMS
	}
	if value > maxRuleEventTimeoutMS {
		return maxRuleEventTimeoutMS
	}
	return value
}

// runBuiltinEvent 执行一个内置事件。非 Windows 平台由 builtin_events_other.go 兜底。
func (s *AppService) runBuiltinEvent(action Action) OperationResult {
	definition, ok := builtinEventByID(strings.TrimSpace(action.RuleEvent))
	if !ok {
		if strings.TrimSpace(action.RuleEvent) == "" {
			return OperationResult{Message: "内置事件未选择事件"}
		}
		return OperationResult{Message: "不支持的内置事件：" + action.RuleEvent}
	}
	switch definition.Needs {
	case "killProcessName":
		if strings.TrimSpace(action.KillProcessName) == "" {
			return OperationResult{Message: "结束指定进程名需要先填写进程名"}
		}
	case "runExePath":
		if strings.TrimSpace(action.RunExePath) == "" {
			return OperationResult{Message: "运行一个应用程序需要先填写程序路径"}
		}
	}
	timeout := builtinEventTimeout(action.RuleEventTimeoutMS)
	if !definition.Timed {
		timeout = 0
	}
	if err := platformRunBuiltinEvent(definition.ID, action, timeout); err != nil {
		s.log("warn", "builtin-event", "内置事件执行失败："+definition.Label, err.Error())
		return OperationResult{Message: err.Error()}
	}
	s.log("info", "builtin-event", "内置事件已执行："+definition.Label, definition.ID)
	if definition.Timed {
		return OperationResult{OK: true, Message: fmt.Sprintf("%s · %d ms", definition.Label, timeout)}
	}
	return OperationResult{OK: true, Message: definition.Label}
}

// lockedKeysFor 返回该锁定事件要屏蔽的按键，以及需要自动按住的按键（0 表示不自动按）。
// 键码是 Windows 虚拟键码。
func lockedKeysFor(eventID string) (blocked []uint16, autoPress uint16, err error) {
	const (
		keyA = 0x41
		keyD = 0x44
		keyS = 0x53
		keyW = 0x57
	)
	switch eventID {
	case RuleEventLockAWSD:
		return []uint16{keyA, keyW, keyS, keyD}, 0, nil
	case RuleEventLockWSD:
		return []uint16{keyW, keyS, keyD}, keyA, nil
	case RuleEventLockASD:
		return []uint16{keyA, keyS, keyD}, keyW, nil
	case RuleEventLockAWD:
		return []uint16{keyW, keyA, keyD}, keyS, nil
	case RuleEventLockAWS:
		return []uint16{keyW, keyA, keyS}, keyD, nil
	default:
		return nil, 0, errors.New("不是锁定类内置事件：" + eventID)
	}
}

// sleepDuration 把毫秒转成 time.Duration，供各平台实现复用。
func sleepDuration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }
