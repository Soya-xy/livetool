/**
 * 内置事件表。
 *
 * 事件 ID 与中文名对照来自《AKA直播》新版前端 i18n（`index.builtInEvent` 一族），
 * 并在主程序 Go 侧 0xF96E00–0xF97800 的 dispatch switch 中逐个比对确认。
 * 顺序与原版下拉一致，不要随意调整。
 *
 * Go 侧的同名定义在 builtin_events.go，两边必须保持一致。
 */
export interface BuiltinEventDefinition {
  /** 事件 ID，与原版 rule_event 取值一致。 */
  id: string
  /** 下拉显示的中文名。 */
  label: string
  /** 是否使用「持续时间(ms)」。 */
  timed?: boolean
  /** 该事件需要额外填写的字段。 */
  needs?: 'killProcessName' | 'runExePath'
  /** 一句话说明，用于界面提示。 */
  detail: string
}

export const BUILTIN_EVENTS: BuiltinEventDefinition[] = [
  { id: 'kill_process_name', label: '结束指定进程名', needs: 'killProcessName', detail: '按进程名结束进程' },
  { id: 'run_exe', label: '运行一个应用程序', needs: 'runExePath', detail: '启动指定路径的程序' },
  { id: 'shutdown', label: '关机', detail: '关闭计算机' },
  { id: 'restart', label: '重启', detail: '重启计算机' },
  { id: 'sleep', label: '睡眠', detail: '让计算机进入睡眠' },
  { id: 'viewlock', label: '锁屏', detail: '锁定当前会话' },
  { id: 'logoff', label: '注销', detail: '注销当前用户' },
  { id: 'minimized', label: '最小化当前窗口', detail: '最小化前台窗口' },
  { id: 'lock_awsd', label: '锁定AWSD(罚站)', timed: true, detail: '屏蔽 W/A/S/D' },
  { id: 'lock_wsd', label: '锁定WSD，自动按A(自动向左)', timed: true, detail: '屏蔽 W/S/D，按住 A' },
  { id: 'lock_asd', label: '锁定ASD，自动按W(自动向前)', timed: true, detail: '屏蔽 A/S/D，按住 W' },
  { id: 'lock_awd', label: '锁定WAD，自动按S(自动向后)', timed: true, detail: '屏蔽 W/A/D，按住 S' },
  { id: 'lock_aws', label: '锁定WAS，自动按D(自动向右)', timed: true, detail: '屏蔽 W/A/S，按住 D' },
  { id: 'cf_quit_room', label: '退出房间(穿越火线)', detail: '向穿越火线前台窗口发送退出房间按键' },
]

export const BUILTIN_EVENT_DEFAULT_TIMEOUT_MS = 1000

export const BUILTIN_EVENT_MAX_TIMEOUT_MS = 600000

export function builtinEventById(id: string): BuiltinEventDefinition | undefined {
  return BUILTIN_EVENTS.find((event) => event.id === id)
}

export function builtinEventLabel(id: string): string {
  return builtinEventById(id)?.label ?? ''
}
