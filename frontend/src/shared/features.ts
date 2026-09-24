export type FeatureValue = string | number | boolean

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
  | 'electronic-woodfish'
  | 'slot-machine'
  | 'gift-screen'
  | 'lottery'
  | 'counter'
  | 'gift-pool'
  | 'screen-lock'
  | 'mosquito-slap'

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
    id: 'component-window', name: '组件窗口', description: '用于显示组件的窗口，如：电子木鱼等', icon: 'Grid', accent: '#ff4650', badge: '组件',
    fields: [text('displayContent', '显示内容', '组件内容', '例如：电子木鱼'), number('width', '窗口宽度', 960, 240, 3840), number('height', '窗口高度', 540, 160, 2160)],
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
    fields: [text('displayContent', '标题', '直播倒计时'), number('durationSeconds', '倒计时(秒)', 60, 1, 86400), { ...number('secondsPerGift', '时间增量(秒)', 10, 0, 86400), giftOnly: true }, select('style', '样式', 'digital', [{ label: '数字', value: 'digital' }, { label: '圆环', value: 'ring' }])], giftRules: true,
  },
  {
    id: 'electronic-woodfish', name: '电子木鱼', description: '电子木鱼，大家一起来集功德', icon: 'Trophy', accent: '#f5a64b', badge: '组件',
    fields: [text('audioPath', '敲击音效', 'fruit.mp3'), number('meritPerClick', '点击功德', 1, 1, 999), { ...number('meritPerGift', '功德增量', 1, 1, 999), giftOnly: true }, toggle('showCounter', '显示计数', true)], giftRules: true,
  },
  {
    id: 'slot-machine', name: '水果机', description: '休闲水果机，水果机盲盒', icon: 'PictureRounded', accent: '#da4da4',
    fields: [text('theme', '主题', 'default'), text('pool', '奖项池', '一等奖, 二等奖, 谢谢参与'), text('weights', '权重', '1, 10, 89'), number('durationMs', '动画时长(ms)', 2200, 300, 600000), toggle('playMusic', '播放背景音效', true)], giftRules: true,
  },
  {
    id: 'gift-screen', name: '礼物飘屏', description: '在直播画面中推送礼物飘屏', icon: 'Tickets', accent: '#111111', badge: '组件',
    fields: [text('imagePath', '飘屏素材', 'images/平底锅.png'), number('durationMs', '持续(ms)', 3500, 100, 600000), number('maxVisible', '最多同时显示', 20, 1, 100)],
    giftRules: true,
  },
  {
    id: 'lottery', name: '大转盘', description: '转盘按照概率抽取奖项', icon: 'DataAnalysis', accent: '#b65ce5', badge: '组件',
    fields: [text('pool', '奖项池', '一等奖, 二等奖, 谢谢参与'), text('weights', '权重', '1, 10, 89'), number('durationMs', '动画时长(ms)', 1700, 300, 600000)],
    giftRules: true,
  },
  {
    id: 'counter', name: '计数器', description: '统计数量，计算数量', icon: 'DataAnalysis', accent: '#1bbd93', badge: '组件',
    fields: [text('displayContent', '显示标题', '礼物计数'), number('initialValue', '初始值', 0, 0, 999999), { ...number('step', '计数增量', 1, 1, 9999), giftOnly: true }],
    giftRules: true,
  },
  {
    id: 'gift-pool', name: '礼物咖', description: '礼物贴纸，玩法菜单设置', icon: 'Tickets', accent: '#f3ae4a', badge: '组件',
    fields: [text('pool', '贴纸池', '啤酒, 小心心, 平底锅'), number('durationMs', '显示(ms)', 3500, 100, 600000), toggle('randomize', '随机素材', true)],
    giftRules: true,
  },
  {
    id: 'screen-lock', name: '屏幕锁键', description: '不同礼物可分别增加空格次数，按完对应次数后解锁', icon: 'Lock', accent: '#3978ef', badge: '组件',
    fields: [
      text('displayContent', '解锁提示', '请按空格解锁', '例如：完成空格输入后解锁'),
      color('lockColor', '锁键颜色', '#e53935'),
      text('lockMediaPath', '锁屏背景素材', '', '通过下方按钮选择图片或视频'),
      { ...number('pressesPerGift', '空格次数', 1, 1, 9999, 1, '该礼物每次增加的空格次数，礼物连击会按数量累计。'), giftOnly: true },
    ],
    giftRules: true,
  },
  {
    id: 'mosquito-slap', name: '拍蚊子', description: '用于显示拍蚊子玩法', icon: 'Pointer', accent: '#3987ed', badge: '动作组件',
    fields: [text('imagePath', '蚊子素材', 'idle.png'), number('durationMs', '游戏时长(ms)', 20000, 1000, 600000), number('score', '命中得分', 1, 1, 999)],
    giftRules: true,
  },
]

export function createDefaultFeatureSettings(input?: Partial<FeatureSettings>): FeatureSettings {
  const settings = {} as FeatureSettings
  for (const feature of FEATURE_DEFINITIONS) {
    const override = input?.[feature.id]
    const values = Object.fromEntries(feature.fields.map((field) => [field.key, field.defaultValue]))
    if (feature.id === 'electronic-woodfish' && override?.values?.meritPerGift === undefined && typeof override?.values?.meritPerClick === 'number') {
      values.meritPerGift = override.values.meritPerClick
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
