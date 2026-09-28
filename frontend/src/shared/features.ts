/** 功能配置值：标量，或礼物菜单这种结构化的数组。 */
export type FeatureValue = string | number | boolean | GiftMenu[]

export type FeatureFieldType = 'text' | 'number' | 'color' | 'boolean' | 'select'

export interface FeatureFieldOption {
  label: string
  value: string
}

export interface FeatureField {
  key: string
  label: string
  type: FeatureFieldType
  defaultValue: FeatureValue
  giftOnly?: boolean
  placeholder?: string
  help?: string
  min?: number
  max?: number
  step?: number
  options?: FeatureFieldOption[]
}

export interface FeatureGiftRule {
  id: string
  giftName: string
  action: string
  enabled: boolean
  values?: Record<string, FeatureValue>
}

/** 礼物菜单里的单个礼物条目：点一下等于收到「礼物名称」这条礼物。 */
export interface GiftMenuItem {
  id: string
  /** 菜单上显示的文字 */
  title: string
  /** 触发规则时使用的礼物名称 */
  giftName: string
}

/** 礼物菜单：菜单标题、样式与礼物列表，对应原版「礼物菜单配置」。 */
export interface GiftMenu {
  id: string
  enabled: boolean
  /** 是否显示左侧标题栏 */
  showLeftTitle: boolean
  title: string
  /** 0.1–1 */
  opacity: number
  fontSize: number
  fontColor: string
  imageSize: number
  gifts: GiftMenuItem[]
}

export function createGiftMenu(index = 1): GiftMenu {
  return {
    id: `menu-${crypto.randomUUID()}`,
    enabled: true,
    showLeftTitle: true,
    title: `礼物菜单${index}`,
    opacity: 1,
    fontSize: 18,
    fontColor: '#e0503f',
    imageSize: 32,
    gifts: [{ id: `gift-${crypto.randomUUID()}`, title: '啤酒', giftName: '啤酒' }],
  }
}

/** 读取配置里的礼物菜单数组，缺失时给一份默认菜单。 */
export function readGiftMenus(values: Record<string, FeatureValue> | undefined): GiftMenu[] {
  const raw = values?.menus
  if (!Array.isArray(raw) || !raw.length) return [createGiftMenu(1)]
  return raw.map((item, index) => {
    const menu = item as Partial<GiftMenu>
    return {
      id: String(menu.id ?? `menu-${index + 1}`),
      enabled: menu.enabled !== false,
      showLeftTitle: menu.showLeftTitle !== false,
      title: String(menu.title ?? `礼物菜单${index + 1}`),
      opacity: typeof menu.opacity === 'number' ? menu.opacity : 1,
      fontSize: typeof menu.fontSize === 'number' ? menu.fontSize : 18,
      fontColor: String(menu.fontColor ?? '#e0503f'),
      imageSize: typeof menu.imageSize === 'number' ? menu.imageSize : 32,
      gifts: Array.isArray(menu.gifts)
        ? menu.gifts.map((gift, giftIndex) => ({
            id: String((gift as GiftMenuItem).id ?? `gift-${giftIndex + 1}`),
            title: String((gift as GiftMenuItem).title ?? ''),
            giftName: String((gift as GiftMenuItem).giftName ?? ''),
          }))
        : [],
    }
  })
}

export interface FeatureConfig {
  enabled: boolean
  values: Record<string, FeatureValue>
  giftRules: FeatureGiftRule[]
}

export type FeatureId =
  | 'green-window'
  | 'component-window'
  | 'virtual-camera'
  | 'speed-curve'
  | 'speed-iba'
  | 'impact-gift'
  | 'frying-pan'
  | 'voice-broadcast'
  | 'danmaku-assistant'
  | 'live-clock'
  | 'gift-screen'
  | 'gift-pool'
  | 'screen-lock'
  // 与原版扩展功能页 1:1 对齐的新增项。
  | 'countdown'
  | 'trash-drop'

export interface FeatureDefinition {
  id: FeatureId
  name: string
  description: string
  icon: string
  accent: string
  badge?: string
  muted?: boolean
  fields: FeatureField[]
  giftRules?: boolean
}

export type FeatureSettings = Record<FeatureId, FeatureConfig>

const text = (key: string, label: string, defaultValue: string, placeholder?: string, help?: string): FeatureField => ({ key, label, type: 'text', defaultValue, placeholder, help })
const number = (key: string, label: string, defaultValue: number, min?: number, max?: number, step?: number, help?: string): FeatureField => ({ key, label, type: 'number', defaultValue, min, max, step, help })
const color = (key: string, label: string, defaultValue: string): FeatureField => ({ key, label, type: 'color', defaultValue })
const toggle = (key: string, label: string, defaultValue: boolean, help?: string): FeatureField => ({ key, label, type: 'boolean', defaultValue, help })
const select = (key: string, label: string, defaultValue: string, options: FeatureFieldOption[]): FeatureField => ({ key, label, type: 'select', defaultValue, options })

export const FEATURE_DEFINITIONS: FeatureDefinition[] = [
  {
    id: 'green-window', name: '绿幕窗口', description: '一个绿色的窗口，用于展示视频、组件等效果', icon: 'VideoCamera', accent: '#0b5d1e',
    fields: [text('displayContent', '显示内容', '视频组件', '例如：视频组件'), text('videoPath', '测试视频', '', '选择素材目录中的视频路径'), number('videoDurationMs', '播放时长(ms)', 8000, 100, 600000), toggle('videoLoop', '循环播放', false), color('backgroundColor', '绿幕颜色', '#00ff00'), number('laneCount', '显示通道', 3, 1, 8)],
  },
  {
    id: 'component-window', name: '组件窗口', description: '用于显示组件的窗口，如：礼物菜单、倒计时等', icon: 'Grid', accent: '#ff4650', badge: '组件',
    fields: [text('displayContent', '显示内容', '组件内容', '例如：礼物菜单'), number('width', '窗口宽度', 960, 240, 3840), number('height', '窗口高度', 540, 160, 2160)],
  },
  {
    id: 'virtual-camera', name: '虚拟摄像头', description: '用于显示组件的摄像头输出', icon: 'Camera', accent: '#22c6c9', badge: 'new',
    fields: [text('deviceName', '设备名称', '阿比虚拟摄像头'), select('resolution', '分辨率', '1920x1080', [{ label: '1920 × 1080', value: '1920x1080' }, { label: '1280 × 720', value: '1280x720' }]), number('fps', '帧率', 30, 1, 60)],
  },
  {
    id: 'speed-curve', name: '加速度(曲线)', description: '通过设置队列到达数量，让事件加速完成', icon: 'Odometer', accent: '#1598dc',
    fields: [number('targetCount', '目标数量', 10, 1, 9999), number('durationMs', '完成时长(ms)', 3000, 100, 600000), select('curve', '曲线', 'ease-out', [{ label: '缓出', value: 'ease-out' }, { label: '线性', value: 'linear' }, { label: '缓入缓出', value: 'ease-in-out' }])], giftRules: true,
  },
  {
    id: 'speed-iba', name: '加速度(IB)', description: '通过设置队列到达数量，让事件加速完成', icon: 'Odometer', accent: '#4d8da8', badge: '组件',
    fields: [number('targetCount', '目标数量', 10, 1, 9999), number('durationMs', '完成时长(ms)', 3000, 100, 600000), number('batchSize', '批量数量', 1, 1, 100)], giftRules: true,
  },
  {
    id: 'impact-gift', name: '砸礼物', description: '收到礼物将礼物砸到屏幕内', icon: 'Present', accent: '#ff8068', badge: '组件', giftRules: true,
    fields: [text('imagePath', '礼物素材', 'images/平底锅.png'), number('count', '数量', 8, 1, 100), number('gravity', '重力', 1800, 0, 5000), number('bounce', '反弹', 0.45, 0, 1, 0.05)],
  },
  {
    id: 'frying-pan', name: '煮播血条', description: '设置煮播血条，增加互动性', icon: 'Dish', accent: '#f6d765',
    fields: [text('displayContent', '显示内容', '煮播血条'), number('maxValue', '血量上限', 100, 1, 99999), { ...number('damagePerGift', '血量变化', 10, 1, 9999), giftOnly: true }, color('barColor', '血条颜色', '#ff4f56')],
    giftRules: true,
  },
  {
    id: 'voice-broadcast', name: '语音播报', description: '朗读直播间弹幕、礼物等信息', icon: 'Microphone', accent: '#f35b9d', muted: true,
    fields: [text('audioPath', '测试音频', 'voices/测试音效.mp3'), text('template', '礼物播报模板', '{user} 送出 {gift}，数量 {count}', '支持 {user}、{text}、{gift}、{count}'), text('chatTemplate', '弹幕播报模板', '{user} 说 {text}', '支持 {user}、{text}、{gift}、{count}'), toggle('chatEnabled', '播报弹幕', true), number('volume', '音量', 0.8, 0, 1, 0.05), toggle('interrupt', '打断当前声音', false)],
    giftRules: true,
  },
  {
    id: 'danmaku-assistant', name: '弹幕助手', description: '查看直播间弹幕、礼物等信息', icon: 'ChatDotRound', accent: '#4edab6', badge: '组件', muted: true,
    fields: [text('keywords', '关键词', '666, 欧皇'), select('keywordMode', '匹配方式', 'contains', [{ label: '包含关键词', value: 'contains' }, { label: '完全一致', value: 'exact' }]), text('replyTemplate', '回复模板', '收到 {user}'), number('cooldownMs', '冷却(ms)', 3000, 0, 600000)],
  },
  {
    id: 'live-clock', name: '加班时钟', description: '直播间下播倒计时，加班时钟', icon: 'AlarmClock', accent: '#30c9d3', badge: '组件',
    fields: [text('displayContent', '标题', '直播倒计时'), number('durationSeconds', '倒计时(秒)', 60, 1, 86400), { ...number('secondsPerGift', '时间增量(秒)', 10, 0, 86400), giftOnly: true }, select('style', '样式', 'digital', [{ label: '数字', value: 'digital' }, { label: '圆环', value: 'ring' }]), text('topText', '顶部文字', ''), text('idleText', '空闲文字', ''), select('skin', '皮肤', '默认', [{ label: '默认', value: '默认' }, { label: '狗子', value: '狗子' }])], giftRules: true,
  },
  {
    id: 'gift-screen', name: '礼物飘屏', description: '在直播画面中推送礼物飘屏', icon: 'Tickets', accent: '#111111', badge: '组件',
    fields: [text('imagePath', '飘屏素材', 'images/平底锅.png'), number('durationMs', '持续(ms)', 3500, 100, 600000), number('maxVisible', '最多同时显示', 20, 1, 100)],
    giftRules: true,
  },
  {
    id: 'gift-pool', name: '礼物菜单', description: '组件窗里的礼物菜单，点一下等于收到该礼物', icon: 'Tickets', accent: '#f3ae4a', badge: '组件',
    // 菜单结构（menus）在设置弹窗里有专门的编辑器，不走通用字段渲染。
    fields: [],
    giftRules: false,
  },
  {
    id: 'screen-lock', name: '屏幕锁键', description: '收到礼物后 3D 锁链锁住画面，按完对应次数空格解锁（全局监听，不独占按键）', icon: 'Lock', accent: '#3978ef', badge: '组件',
    fields: [
      text('displayContent', '解锁提示', '请按空格解锁', '显示在锁链中间的面板标题'),
      color('lockColor', '锁链颜色', '#e53935'),
      text('lockMediaPath', '锁屏背景素材', '', '通过下方按钮选择图片或视频'),
      number('lockSoundVolume', '音效音量', 0.8, 0, 1, 0.05, '0–1，仅在填写了音效素材时生效。'),
      text('lockOpenSound', '开链音效', '', '素材目录内的音频路径，例如 voices/铁链摩擦.mp3；留空不播放'),
      text('lockHitSound', '敲击音效', '', '素材目录内的音频路径，例如 voices/打铁.mp3；留空不播放'),
      number('lockBlurMax', '边缘模糊', 0, 0, 40, 1, '0 = 关闭（与原版一致），数值越大画面四角越虚。'),
      { ...number('pressesPerGift', '空格次数', 1, 1, 9999, 1, '该礼物每次增加的空格次数，礼物连击会按数量累计。'), giftOnly: true },
    ],
    giftRules: true,
  },
  {
    id: 'countdown', name: '倒计时', description: '倒计时，是真的很不错', icon: 'Timer', accent: '#e0603c', badge: '组件',
    fields: [
      color('bgColor1', '背景颜色1', '#1a1a2e'),
      color('bgColor2', '背景颜色2', '#16213e'),
      color('openColor', '开启颜色', '#00ff88'),
      color('closeColor', '关闭颜色', '#ff4650'),
      color('countdownColor', '倒计时颜色', '#ffffff'),
      text('bgImage', '背景图片', '', '素材目录内的图片路径，留空使用渐变色'),
      number('durationSeconds', '默认时间(秒)', 60, 1, 86400),
      toggle('tempEnabled', '温度设置', false),
      number('tempMin', '最低温度', 0, -50, 200),
      number('tempMax', '最高温度', 100, -50, 500),
    ],
    giftRules: true,
  },
  {
    id: 'trash-drop', name: '垃圾掉落', description: '收到礼物掉落垃圾，堆满垃圾桶', icon: 'Delete', accent: '#8a6d3b', badge: 'new',
    fields: [
      text('imagePath', '垃圾素材', 'images/垃圾.png', '留空时组件窗绘制纯 CSS 垃圾桶'),
      text('binPath', '垃圾桶素材', '', '可选，留空使用内置垃圾桶'),
      number('count', '掉落数量', 1, 1, 100),
      number('maxVisible', '最多显示', 60, 1, 200),
      number('durationMs', '停留(ms)', 4000, 100, 600000),
    ],
    giftRules: true,
  },
]

export function createDefaultFeatureSettings(input?: Partial<FeatureSettings>): FeatureSettings {
  const settings = {} as FeatureSettings
  for (const feature of FEATURE_DEFINITIONS) {
    const override = input?.[feature.id]
    const values = Object.fromEntries(feature.fields.map((field) => [field.key, field.defaultValue]))
    if (feature.id === 'gift-pool') {
      values.menus = override?.values?.menus ?? [createGiftMenu(1)]
    }
    settings[feature.id] = {
      enabled: override?.enabled ?? false,
      values: { ...values, ...(override?.values ?? {}) },
      giftRules: override?.giftRules ? override.giftRules.map((rule) => ({ ...rule, values: { ...(rule.values ?? {}) } })) : defaultGiftRules(feature),
    }
    if (feature.id === 'green-window' || feature.id === 'component-window') {
      settings[feature.id].values.alwaysOnTop = false
    }
  }
  return settings
}

function defaultGiftRules(feature: FeatureDefinition): FeatureGiftRule[] {
  if (!feature.giftRules) return []
  return [{ id: `${feature.id}-default-rule`, giftName: '啤酒', action: feature.id === 'frying-pan' ? '减少' : '增加', enabled: true, values: {} }]
}
