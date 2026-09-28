//go:build !windows

package main

import "errors"

// platformRunBuiltinEvent 在非 Windows 平台统一返回明确错误。
// 内置事件全部依赖 Windows 系统能力（关机 / 锁屏 / 结束进程 / 键盘钩子），
// 复刻版不在其它平台上假装执行成功。
func platformRunBuiltinEvent(eventID string, action Action, timeout int) error {
	return errors.New("内置事件「" + eventID + "」仅支持 Windows")
}
