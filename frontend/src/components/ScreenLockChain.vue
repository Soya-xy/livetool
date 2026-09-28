<script setup lang="ts">
/**
 * 屏幕锁键的 Three.js 3D 锁链特效。
 *
 * 移植自原版「屏幕锁链」组件：链条、颜色、尺寸全部由代码程序化生成，不需要任何图片素材。
 *  - 两条链条分别沿屏幕的两条对角线摆放，交叉锁在画面中间；
 *  - 收到礼物时沿对角线方向「缠绕」展开（1.85s easeOutCubic），同时播放开链音效；
 *  - 每按一次空格：多频正弦叠加的震动 + 打铁音效 + 计数面板弹一下；
 *  - 次数归零：放大 1.06 + 模糊 10px + 淡出（420ms）。
 * 金属质感来自程序化 PMREM 环境贴图 + MeshPhysicalMaterial(metalness:1, clearcoat:1) + 自定义流光 shader。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  ACESFilmicToneMapping,
  AdditiveBlending,
  AmbientLight,
  CanvasTexture,
  CatmullRomCurve3,
  Color,
  DirectionalLight,
  DynamicDrawUsage,
  Group,
  HemisphereLight,
  InstancedMesh,
  Mesh,
  MeshBasicMaterial,
  MeshPhysicalMaterial,
  NormalBlending,
  Object3D,
  OrthographicCamera,
  PCFShadowMap,
  PerspectiveCamera,
  Plane,
  PlaneGeometry,
  PMREMGenerator,
  Quaternion,
  RGBAFormat,
  Scene,
  ShaderMaterial,
  SphereGeometry,
  SRGBColorSpace,
  TubeGeometry,
  Vector2,
  Vector3,
  WebGLRenderer,
  WebGLRenderTarget,
} from 'three'

interface Props {
  /** 锁定时为 true（挂载后一直保持挂载，靠该字段切换显隐，避免重复创建 WebGL 上下文）。 */
  active?: boolean
  /** 解锁完成的离场动画。 */
  leaving?: boolean
  /** 剩余空格次数，也就是面板上的大数字。 */
  count?: number
  /** 面板标题。 */
  title?: string
  /** 气泡消息（只显示最后一条）。 */
  message?: string
  /** 链条颜色，支持 #RRGGBB、0xRRGGBB 与 CSS 颜色名。 */
  color?: string
  /** 层数从 0 变正数时播放的音效（素材地址，可留空）。 */
  openSound?: string
  /** 敲击解锁时播放的音效（素材地址，可留空）。 */
  hitSound?: string
  /** 音效音量 0–1。 */
  volume?: number
  /** 链条松弛度 0.6–1.2，越大链条越长。 */
  link?: number
  /** 松弛度微调（0.001 步长）。 */
  linkFine?: number
  /** 边缘径向模糊强度，0 表示关闭（与原版默认一致）。 */
  blurMax?: number
}

interface RevealUniforms {
  uRevealT: { value: number }
  uRevealStart: { value: number }
  uRevealEnd: { value: number }
  uRevealDir: { value: Vector3 }
  uRevealSoft: { value: number }
}

interface ScreenFrame {
  spanA: number
  spanB: number
  quatA: Quaternion
  quatB: Quaternion
  center: Vector3
  axis: Vector3
  topLeft: Vector3
  topRight: Vector3
  bottomLeft: Vector3
  bottomRight: Vector3
  dirA: Vector3
  dirB: Vector3
}

interface PostPipeline {
  target: WebGLRenderTarget
  scene: Scene
  camera: OrthographicCamera
  material: ShaderMaterial
  resize: (width: number, height: number, pixelRatio: number) => void
}

interface ChainOptions {
  baseQuat: Quaternion
  zLift: number
  sheenDir: Vector3
  phase: number
  count: number
  pitch: number
  reveal: { uniforms: RevealUniforms; startPos: Vector3 }
}

const props = withDefaults(defineProps<Props>(), {
  active: false,
  leaving: false,
  count: 0,
  title: '请按空格解锁',
  message: '',
  color: '#e53935',
  openSound: '',
  hitSound: '',
  volume: 0.8,
  link: 0.92,
  linkFine: 0,
  blurMax: 0,
})

const REVEAL_SECONDS = 1.85
const LEAVE_SECONDS = 0.42
const PITCH_RATIO = 8.8
const MIN_LINKS = 18
const MAX_LINKS = 110
const DEFAULT_CHAIN_COLOR = 0xaa5eca

const container = ref<HTMLDivElement | null>(null)
const isShown = ref(false)
const isLeaving = ref(false)
const isBumping = ref(false)
const bumpKey = ref(0)
const bubbleText = ref('')
const bubbleKey = ref(0)

let renderer: WebGLRenderer | null = null
let scene: Scene | null = null
let camera: PerspectiveCamera | null = null
let chainGroup: Group | null = null
let chainA: Group | null = null
let chainB: Group | null = null
let glow: Mesh | null = null
let pmrem: PMREMGenerator | null = null
let environment: { dispose: () => void } | null = null
let post: PostPipeline | null = null
let frame: ScreenFrame | null = null
let tubeGeometry: TubeGeometry | null = null
let resizeObserver: ResizeObserver | null = null
let rafId = 0
let revealStart = 0
let lastFrameTime = 0
let running = false
let bumpTimer: ReturnType<typeof setTimeout> | undefined
let leaveTimer: ReturnType<typeof setTimeout> | undefined
let lastMessage = ''
let audio: HTMLAudioElement | null = null

const energy = { value: 0, phase: 0, lastPressMs: 0 }
const sheen = { uTime: { value: 0 }, uSheenStrength: { value: 0.65 } }
const scratch = {
  screenX: new Vector3(),
  quatAxis: new Quaternion(),
  quatScreen: new Quaternion(),
  swayA: new Quaternion(),
  swayB: new Quaternion(),
}

const geometryScale = computed(() => {
  const link = Number.isFinite(props.link) ? Math.max(0.6, Math.min(1.2, props.link)) : 0.92
  const fine = Number.isFinite(props.linkFine) ? Math.max(-80, Math.min(80, props.linkFine)) : 0
  return Math.max(0.6, Math.min(1.2, link + fine * 0.001))
})

const chainColor = computed(() => parseColor(props.color))

const rootClass = computed(() => ({
  active: isShown.value && !isLeaving.value,
  leaving: isLeaving.value,
}))

function parseColor(value: unknown): Color {
  const raw = String(value ?? '').trim()
  if (!raw) return new Color(DEFAULT_CHAIN_COLOR)
  if (/^0x[0-9a-f]{6}$/i.test(raw)) return new Color(Number.parseInt(raw.slice(2), 16))
  try {
    return new Color(raw)
  } catch {
    return new Color(DEFAULT_CHAIN_COLOR)
  }
}

function buildCurve(width: number, height: number, radius: number): CatmullRomCurve3 {
  const halfWidth = width / 2
  const halfHeight = height / 2
  const r = Math.min(radius, halfWidth, halfHeight)
  const points: Vector3[] = []
  const arc = (cx: number, cy: number, from: number, to: number, segments = 10): void => {
    for (let index = 0; index <= segments; index += 1) {
      const angle = from + (to - from) * (index / segments)
      points.push(new Vector3(cx + Math.cos(angle) * r, cy + Math.sin(angle) * r, 0))
    }
  }
  arc(halfWidth - r, halfHeight - r, 0, Math.PI * 0.5)
  arc(-halfWidth + r, halfHeight - r, Math.PI * 0.5, Math.PI)
  arc(-halfWidth + r, -halfHeight + r, Math.PI, Math.PI * 1.5)
  arc(halfWidth - r, -halfHeight + r, Math.PI * 1.5, Math.PI * 2)
  return new CatmullRomCurve3(points, true, 'catmullrom', 0.02)
}

/** 往标准材质里插代码：缠绕裁剪 + 沿链条流动的金属高光。 */
function patchMaterial(material: MeshPhysicalMaterial, sheenDir: Vector3, reveal?: RevealUniforms): void {
  material.onBeforeCompile = (shader) => {
    shader.uniforms.uTime = sheen.uTime
    shader.uniforms.uSheenStrength = sheen.uSheenStrength
    shader.uniforms.uSheenDir = { value: sheenDir.clone().normalize() }
    if (reveal) {
      shader.uniforms.uRevealT = reveal.uRevealT
      shader.uniforms.uRevealStart = reveal.uRevealStart
      shader.uniforms.uRevealEnd = reveal.uRevealEnd
      shader.uniforms.uRevealDir = reveal.uRevealDir
      shader.uniforms.uRevealSoft = reveal.uRevealSoft
    }
    shader.vertexShader = `
      varying vec3 vSheenWorldPos;
    ` + shader.vertexShader
    shader.vertexShader = shader.vertexShader.replace('#include <worldpos_vertex>', `
      #include <worldpos_vertex>
      #ifdef USE_INSTANCING
        vSheenWorldPos = ( modelMatrix * instanceMatrix * vec4( transformed, 1.0 ) ).xyz;
      #else
        vSheenWorldPos = ( modelMatrix * vec4( transformed, 1.0 ) ).xyz;
      #endif
    `)
    shader.fragmentShader = `
      uniform float uTime;
      uniform float uSheenStrength;
      uniform vec3 uSheenDir;
      uniform float uRevealT;
      uniform float uRevealStart;
      uniform float uRevealEnd;
      uniform vec3 uRevealDir;
      uniform float uRevealSoft;
      varying vec3 vSheenWorldPos;
    ` + shader.fragmentShader
    shader.fragmentShader = shader.fragmentShader.replace('vec4 diffuseColor = vec4( diffuse, opacity );', `
      vec4 diffuseColor = vec4( diffuse, opacity );
      float revealT = clamp( uRevealT, 0.0, 1.0 );
      float revealProj = dot( vSheenWorldPos, normalize( uRevealDir ) );
      float revealPos = mix( uRevealStart, uRevealEnd, revealT );
      float revealHead = smoothstep( uRevealStart - uRevealSoft, uRevealStart + uRevealSoft, revealProj );
      float revealTail = 1.0 - smoothstep( revealPos - uRevealSoft, revealPos + uRevealSoft, revealProj );
      diffuseColor.a *= revealHead * revealTail;
    `)
    shader.fragmentShader = shader.fragmentShader.replace('#include <lights_physical_fragment>', `
      #include <lights_physical_fragment>
      float sheenPos = dot( vSheenWorldPos, uSheenDir );
      float sheenPhase = fract( sheenPos * 0.055 + uTime * 0.12 );
      float sheenBand = exp( -pow( ( sheenPhase - 0.5 ) / 0.06, 2.0 ) );
      float sheenFresnel = pow( 1.0 - saturate( dot( normalize( vNormal ), normalize( vViewPosition ) ) ), 3.2 );
      vec3 sheenAdd = vec3( 1.0 ) * sheenBand * sheenFresnel * uSheenStrength;
      reflectedLight.directSpecular += sheenAdd;
      reflectedLight.indirectSpecular += sheenAdd * 0.55;
    `)
  }
  material.needsUpdate = true
}

/** 把 NDC 四个角投影到「过原点、垂直于视线」的平面上，得到屏幕在对角线方向上的跨度与朝向。 */
function measureScreen(): ScreenFrame {
  const view = camera as PerspectiveCamera
  const axis = new Vector3()
  view.getWorldDirection(axis)
  const plane = new Plane().setFromNormalAndCoplanarPoint(axis, new Vector3(0, 0, 0))
  const unproject = (x: number, y: number): Vector3 => {
    const direction = new Vector3(x, y, 0.5).unproject(view).sub(view.position).normalize()
    const denominator = plane.normal.dot(direction)
    const distance = -(plane.normal.dot(view.position) + plane.constant) / denominator
    return view.position.clone().add(direction.multiplyScalar(distance))
  }
  const topLeft = unproject(-1, 1)
  const topRight = unproject(1, 1)
  const bottomRight = unproject(1, -1)
  const bottomLeft = unproject(-1, -1)
  const diagonalA = bottomRight.clone().sub(topLeft)
  const diagonalB = bottomLeft.clone().sub(topRight)
  const center = topLeft.clone().add(bottomRight).multiplyScalar(0.5)
  const dirA = diagonalA.clone().normalize()
  const dirB = diagonalB.clone().normalize()
  return {
    spanA: diagonalA.length(),
    spanB: diagonalB.length(),
    quatA: new Quaternion().setFromUnitVectors(new Vector3(1, 0, 0), dirA),
    quatB: new Quaternion().setFromUnitVectors(new Vector3(1, 0, 0), dirB),
    center,
    axis: axis.clone(),
    topLeft,
    topRight,
    bottomLeft,
    bottomRight,
    dirA,
    dirB,
  }
}

function linkCount(span: number, pitch: number): number {
  return Math.max(MIN_LINKS, Math.min(MAX_LINKS, Math.ceil((span + pitch * 12) / pitch) + 1))
}

function buildChain(options: ChainOptions): Group {
  const scale = geometryScale.value
  const group = new Group()
  group.quaternion.copy(options.baseQuat)
  group.position.copy(frame!.center).addScaledVector(frame!.axis, options.zLift)

  const material = new MeshPhysicalMaterial({
    color: chainColor.value,
    metalness: 1,
    roughness: 0.12,
    clearcoat: 1,
    clearcoatRoughness: 0.06,
    reflectivity: 1,
    ior: 1.5,
    envMapIntensity: 1.35,
    emissive: new Color(0x0b0d10),
    emissiveIntensity: 0.06,
  })
  patchMaterial(material, options.sheenDir, options.reveal.uniforms)

  // 主链环：同一个圆角矩形管体沿切线按 pitch 摆放，相邻 90° 交替 → 标准链条外观。
  const links = new InstancedMesh(tubeGeometry as TubeGeometry, material, options.count)
  links.castShadow = true
  links.receiveShadow = true
  links.instanceMatrix.setUsage(DynamicDrawUsage)
  const dummy = new Object3D()
  for (let index = 0; index < options.count; index += 1) {
    const offset = index - (options.count - 1) * 0.5
    const odd = index % 2 === 0 ? 0 : 1
    dummy.position.set(offset * options.pitch, 0, odd ? 0.55 * scale : -0.55 * scale)
    dummy.rotation.set(odd ? Math.PI * 0.5 : 0, 0, odd ? Math.PI * 0.5 : 0)
    dummy.rotation.y += odd ? 0.16 : -0.12
    dummy.rotation.x += odd ? -0.08 : 0.06
    dummy.updateMatrix()
    links.setMatrixAt(index, dummy.matrix)
  }
  links.instanceMatrix.needsUpdate = true
  group.add(links)

  // 连接销：链环关节处的小销子。
  const pinGeometry = new SphereGeometry(0.32 * scale, 10, 10)
  const pinMaterial = new MeshPhysicalMaterial({
    color: new Color(0xf4f7fb),
    metalness: 1,
    roughness: 0.22,
    clearcoat: 0.9,
    clearcoatRoughness: 0.14,
    reflectivity: 1,
    ior: 1.5,
  })
  patchMaterial(pinMaterial, options.sheenDir, options.reveal.uniforms)
  const pins = new InstancedMesh(pinGeometry, pinMaterial, options.count)
  pins.castShadow = true
  pins.receiveShadow = true
  const pinDummy = new Object3D()
  for (let index = 0; index < options.count; index += 1) {
    const offset = index - (options.count - 1) * 0.5
    const odd = index % 2 === 0 ? 0 : 1
    const angle = (0.9 + ((index * 1.7) % 1.9)) * Math.PI
    const radius = odd ? 5.5 : 7
    pinDummy.position.set(
      offset * options.pitch + Math.cos(angle) * radius * 0.12 * scale,
      Math.sin(angle) * radius * 0.1 * scale,
      odd ? 1.3 * scale : -1.3 * scale,
    )
    pinDummy.updateMatrix()
    pins.setMatrixAt(index, pinDummy.matrix)
  }
  pins.instanceMatrix.needsUpdate = true
  group.add(pins)

  group.userData.phase = options.phase
  group.userData.baseQuat = options.baseQuat.clone()
  group.userData.revealUniforms = options.reveal.uniforms
  group.userData.mainMaterial = material
  group.userData.basePos = group.position.clone()
  group.userData.startPos = options.reveal.startPos.clone().addScaledVector(frame!.axis, options.zLift)
  return group
}

function disposeChain(target: Group | null): void {
  target?.traverse((child) => {
    const mesh = child as Mesh
    if (mesh.geometry && mesh.geometry !== tubeGeometry) mesh.geometry.dispose()
    if (mesh.material) {
      if (Array.isArray(mesh.material)) mesh.material.forEach((item) => item.dispose())
      else mesh.material.dispose()
    }
  })
}

function rebuildChains(): void {
  if (!scene || !chainGroup || !camera) return
  if (chainA) chainGroup.remove(chainA)
  if (chainB) chainGroup.remove(chainB)
  disposeChain(chainA)
  disposeChain(chainB)
  chainA = null
  chainB = null

  const scale = geometryScale.value
  tubeGeometry?.dispose()
  const curve = buildCurve(14.2 * scale, 8.2 * scale, 4.1 * scale)
  tubeGeometry = new TubeGeometry(curve, 220, 1.08 * scale, 18, true)
  tubeGeometry.computeVertexNormals()

  frame = measureScreen()
  const pitch = PITCH_RATIO * scale
  const countA = linkCount(frame.spanA, pitch)
  const countB = linkCount(frame.spanB, pitch)

  const revealA: RevealUniforms = {
    uRevealT: { value: 1 },
    uRevealStart: { value: frame.topLeft.dot(frame.dirA) },
    uRevealEnd: { value: frame.bottomRight.dot(frame.dirA) },
    uRevealDir: { value: frame.dirA.clone() },
    uRevealSoft: { value: 1.6 * scale },
  }
  const revealB: RevealUniforms = {
    uRevealT: { value: 1 },
    uRevealStart: { value: frame.topRight.dot(frame.dirB) },
    uRevealEnd: { value: frame.bottomLeft.dot(frame.dirB) },
    uRevealDir: { value: frame.dirB.clone() },
    uRevealSoft: { value: 1.6 * scale },
  }

  chainB = buildChain({ baseQuat: frame.quatB, zLift: -1.1, sheenDir: new Vector3(-1, 1, 0.4), phase: 0, count: countB, pitch, reveal: { uniforms: revealB, startPos: frame.topRight } })
  chainA = buildChain({ baseQuat: frame.quatA, zLift: 1.1, sheenDir: new Vector3(1, 1, 0.2), phase: 0.33, count: countA, pitch, reveal: { uniforms: revealA, startPos: frame.topLeft } })
  chainGroup.add(chainB)
  chainGroup.add(chainA)

  if (glow) glow.position.copy(frame.center).addScaledVector(frame.axis, 3.4 * scale)
}

/** 程序化「摄影棚」环境贴图：金属反光全靠它。 */
function buildEnvironment(): void {
  const studio = new Scene()
  studio.background = new Color(0x7b7b7b)
  const lightA = new DirectionalLight(0xffffff, 2)
  lightA.position.set(-1, 1, 1)
  const lightB = new DirectionalLight(0xffffff, 1.2)
  lightB.position.set(1, 0.4, 0.6)
  studio.add(lightA)
  studio.add(lightB)
  studio.add(new AmbientLight(0xffffff, 0.5))
  const panel = new Mesh(new PlaneGeometry(8, 5), new MeshBasicMaterial({ color: 0xffffff }))
  panel.position.set(-2, 2.5, 1.5)
  panel.rotation.y = 0.6
  panel.rotation.x = -0.25
  studio.add(panel)
  const target = (pmrem as PMREMGenerator).fromScene(studio, 0.03)
  studio.traverse((child) => {
    const mesh = child as Mesh
    mesh.geometry?.dispose()
    if (mesh.material) (mesh.material as MeshBasicMaterial).dispose()
  })
  if (scene) scene.environment = target.texture
  environment = target
}

function buildGlow(): Mesh {
  const scale = geometryScale.value
  const canvas = document.createElement('canvas')
  canvas.width = 256
  canvas.height = 256
  const context = canvas.getContext('2d') as CanvasRenderingContext2D
  const gradient = context.createRadialGradient(110, 95, 0, 128, 128, 120)
  gradient.addColorStop(0, 'rgba(255,255,255,0.85)')
  gradient.addColorStop(0.32, 'rgba(255,255,255,0.22)')
  gradient.addColorStop(1, 'rgba(255,255,255,0)')
  context.fillStyle = gradient
  context.fillRect(0, 0, 256, 256)
  const texture = new CanvasTexture(canvas)
  texture.colorSpace = SRGBColorSpace
  return new Mesh(
    new PlaneGeometry(22 * scale, 22 * scale),
    new MeshBasicMaterial({ map: texture, transparent: true, opacity: 0.35, blending: AdditiveBlending, depthWrite: false }),
  )
}

/** 中心清晰、四角发虚的径向模糊；blurMax 为 0 时退化为直接渲染。 */
function buildPost(): PostPipeline {
  const rendererRef = renderer as WebGLRenderer
  const pixelRatio = rendererRef.getPixelRatio()
  const size = new Vector2(Math.max(1, Math.floor(1 * pixelRatio)), Math.max(1, Math.floor(1 * pixelRatio)))
  const target = new WebGLRenderTarget(size.x, size.y, { samples: 0, format: RGBAFormat })
  target.texture.colorSpace = SRGBColorSpace
  const quadScene = new Scene()
  const quadCamera = new OrthographicCamera(-1, 1, 1, -1, 0, 1)
  const quadMaterial = new ShaderMaterial({
    uniforms: {
      tColor: { value: target.texture },
      uResolution: { value: new Vector2(size.x, size.y) },
      uBlurMax: { value: 0 },
    },
    vertexShader: `
      varying vec2 vUv;
      void main() {
        vUv = uv;
        gl_Position = vec4( position.xy, 0.0, 1.0 );
      }
    `,
    fragmentShader: `
      precision highp float;
      varying vec2 vUv;
      uniform sampler2D tColor;
      uniform vec2 uResolution;
      uniform float uBlurMax;

      void main() {
        float aspect = uResolution.x / max( 1.0, uResolution.y );
        vec2 d = vUv - vec2( 0.5 );
        d.x *= aspect;
        float dist = length( d );
        float blur = smoothstep( 0.12, 0.62, dist );
        float r = blur * uBlurMax;
        vec2 px = 1.0 / uResolution;
        vec4 c = texture2D( tColor, vUv ) * 0.28;
        c += texture2D( tColor, vUv + vec2(  1.0,  0.0 ) * px * r ) * 0.14;
        c += texture2D( tColor, vUv + vec2( -1.0,  0.0 ) * px * r ) * 0.14;
        c += texture2D( tColor, vUv + vec2(  0.0,  1.0 ) * px * r ) * 0.14;
        c += texture2D( tColor, vUv + vec2(  0.0, -1.0 ) * px * r ) * 0.14;
        c += texture2D( tColor, vUv + vec2(  1.0,  1.0 ) * px * r ) * 0.04;
        c += texture2D( tColor, vUv + vec2( -1.0,  1.0 ) * px * r ) * 0.04;
        c += texture2D( tColor, vUv + vec2(  1.0, -1.0 ) * px * r ) * 0.04;
        c += texture2D( tColor, vUv + vec2( -1.0, -1.0 ) * px * r ) * 0.04;
        gl_FragColor = vec4( c.rgb, c.a );
      }
    `,
    depthTest: false,
    depthWrite: false,
    transparent: true,
    blending: NormalBlending,
  })
  quadScene.add(new Mesh(new PlaneGeometry(2, 2), quadMaterial))
  return {
    target,
    scene: quadScene,
    camera: quadCamera,
    material: quadMaterial,
    resize: (width, height, ratio) => {
      const pixelWidth = Math.max(1, Math.floor(width * ratio))
      const pixelHeight = Math.max(1, Math.floor(height * ratio))
      target.setSize(pixelWidth, pixelHeight)
      quadMaterial.uniforms.uResolution.value.set(pixelWidth, pixelHeight)
    },
  }
}

function buildScene(): void {
  const host = container.value
  if (!host || renderer) return

  renderer = new WebGLRenderer({ antialias: true, alpha: true, powerPreference: 'high-performance' })
  renderer.outputColorSpace = SRGBColorSpace
  renderer.toneMapping = ACESFilmicToneMapping
  renderer.toneMappingExposure = 1.45
  renderer.setClearColor(0xffffff, 0)
  renderer.shadowMap.enabled = true
  renderer.shadowMap.type = PCFShadowMap
  host.appendChild(renderer.domElement)

  scene = new Scene()
  camera = new PerspectiveCamera(82, 1, 0.1, 300)
  camera.position.set(0, -30, 78)
  camera.lookAt(0, 0, 0)

  chainGroup = new Group()
  scene.add(chainGroup)

  scene.add(new AmbientLight(0xffffff, 0.75))
  const sky = new HemisphereLight(0xffffff, 0x4a4a4a, 0.55)
  sky.position.set(0, 20, 40)
  scene.add(sky)

  const key = new DirectionalLight(0xffffff, 3.1)
  key.position.set(-45, 52, 72)
  key.castShadow = true
  key.shadow.mapSize.set(2048, 2048)
  key.shadow.camera.near = 1
  key.shadow.camera.far = 240
  key.shadow.camera.left = -80
  key.shadow.camera.right = 80
  key.shadow.camera.top = 80
  key.shadow.camera.bottom = -80
  key.shadow.bias = -0.0015
  key.shadow.normalBias = 0.02
  scene.add(key)

  const fill = new DirectionalLight(0xffffff, 1.25)
  fill.position.set(55, -35, 40)
  scene.add(fill)

  pmrem = new PMREMGenerator(renderer)
  buildEnvironment()

  glow = buildGlow()
  chainGroup.add(glow)

  post = buildPost()
  post.material.uniforms.uBlurMax.value = Math.max(0, Number(props.blurMax) || 0)
}

function resize(): void {
  const host = container.value
  if (!host || !renderer || !camera || !post) return
  const rect = host.getBoundingClientRect()
  const width = Math.max(1, Math.floor(rect.width))
  const height = Math.max(1, Math.floor(rect.height))
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
  renderer.setSize(width, height, false)
  camera.aspect = width / height
  camera.updateProjectionMatrix()
  post.resize(width, height, renderer.getPixelRatio())
  rebuildChains()
  if (glow && frame) glow.position.copy(frame.center).addScaledVector(frame.axis, 3.4 * geometryScale.value)
}

function renderFrame(elapsed: number): void {
  if (!renderer || !scene || !camera || !chainGroup || !chainA || !chainB || !glow || !post || !frame) return
  sheen.uTime.value = elapsed
  const delta = Math.max(0, Math.min(0.05, elapsed - lastFrameTime))
  lastFrameTime = elapsed

  // 待机：整组链条上下浮动，周期 4 秒。
  chainGroup.position.set(0, Math.sin((elapsed * Math.PI * 2) / 4) * 0.7, 0)
  chainGroup.quaternion.identity()

  // 敲击震动：按键越快能量越高，三组正弦叠加出抖动。
  if (energy.value > 1e-4) {
    energy.value *= Math.exp(-delta * 6.2)
    energy.phase += delta * (18 + 28 * energy.value)
    const shakeX = (Math.sin(energy.phase * 1.9) + Math.sin(energy.phase * 3.1)) * 0.5
    const shakeY = (Math.sin(energy.phase * 2.3) + Math.sin(energy.phase * 4.3)) * 0.5
    const shakeZ = (Math.sin(energy.phase * 2.9) + Math.sin(energy.phase * 5.7)) * 0.5
    const amplitude = 0.42 * energy.value
    const angle = (Math.PI / 180) * 1.3 * energy.value
    chainGroup.position.x += shakeX * amplitude
    chainGroup.position.y += shakeY * amplitude * 0.7
    chainGroup.position.z += shakeZ * amplitude * 0.35
    scratch.screenX.set(1, 0, 0).applyQuaternion(camera.quaternion)
    scratch.quatAxis.setFromAxisAngle(frame.axis, shakeX * angle)
    scratch.quatScreen.setFromAxisAngle(scratch.screenX, shakeY * angle * 0.8)
    chainGroup.quaternion.multiply(scratch.quatAxis).multiply(scratch.quatScreen)
  } else {
    energy.value = 0
  }

  // 缠绕出现：沿对角线把链条「铺」进画面，同时从起始角滑到中心。
  const progress = Math.min(1, Math.max(0, (elapsed - revealStart) / REVEAL_SECONDS))
  const eased = 1 - Math.pow(1 - progress, 3)
  for (const chain of [chainA, chainB]) {
    const uniforms = chain.userData.revealUniforms as RevealUniforms
    uniforms.uRevealT.value = eased
    chain.position.lerpVectors(chain.userData.startPos as Vector3, chain.userData.basePos as Vector3, eased)
  }

  // 待机摆动：两条链绕自身轴反向轻微摆动（周期 7s / 9s）。
  scratch.swayA.setFromAxisAngle(frame.axis, Math.sin((elapsed * Math.PI * 2) / 7 + (chainA.userData.phase as number)) * (Math.PI / 180) * 1.5)
  scratch.swayB.setFromAxisAngle(frame.axis, Math.sin((elapsed * Math.PI * 2) / 9 + (chainB.userData.phase as number)) * (Math.PI / 180) * -1.5)
  chainA.quaternion.copy(chainA.userData.baseQuat as Quaternion).multiply(scratch.swayA)
  chainB.quaternion.copy(chainB.userData.baseQuat as Quaternion).multiply(scratch.swayB)

  // 中心光晕脉动，周期 3 秒。
  const pulse = 0.5 + 0.5 * Math.sin((elapsed * Math.PI * 2) / 3)
  ;(glow.material as MeshBasicMaterial).opacity = 0.2 + 0.3 * pulse
  glow.scale.setScalar(0.98 + 0.04 * pulse)

  if (post.material.uniforms.uBlurMax.value > 0) {
    renderer.setRenderTarget(post.target)
    renderer.render(scene, camera)
    renderer.setRenderTarget(null)
    renderer.render(post.scene, post.camera)
  } else {
    renderer.render(scene, camera)
  }
}

function loop(): void {
  if (!running) return
  renderFrame(performance.now() / 1000)
  rafId = requestAnimationFrame(loop)
}

function startLoop(): void {
  if (running) return
  running = true
  lastFrameTime = performance.now() / 1000
  rafId = requestAnimationFrame(loop)
}

function stopLoop(): void {
  running = false
  if (rafId) cancelAnimationFrame(rafId)
  rafId = 0
}

function playSound(path: unknown): void {
  const url = String(path ?? '').trim()
  if (!url) return
  try {
    audio ??= new Audio()
    audio.volume = Math.max(0, Math.min(1, Number(props.volume) || 0))
    audio.src = url
    audio.currentTime = 0
    void audio.play().catch(() => undefined)
  } catch {
    // 播放失败（素材缺失 / 自动播放被拦截）时静默忽略，不影响特效。
  }
}

function bump(): void {
  bumpKey.value += 1
  isBumping.value = true
  if (bumpTimer) clearTimeout(bumpTimer)
  bumpTimer = setTimeout(() => {
    isBumping.value = false
    bumpTimer = undefined
  }, 240)
}

/** 空格按下：打铁音效 + 震动能量（父组件在按键或次数减少时调用）。 */
function triggerHit(force = false): void {
  if (force || Number(props.count) >= 1) playSound(props.hitSound)
  const now = performance.now()
  const delta = energy.lastPressMs ? (now - energy.lastPressMs) / 1000 : 10
  energy.lastPressMs = now
  const rate = 1 / Math.max(0.12, delta)
  energy.value = Math.min(3.2, energy.value + Math.max(0.22, Math.min(1.4, 0.22 + rate * 0.14)))
}

function setShown(shown: boolean): void {
  if (leaveTimer) {
    clearTimeout(leaveTimer)
    leaveTimer = undefined
  }
  if (shown) {
    // 首次锁定才创建 WebGL 场景：没用到锁键的窗口不用为特效付出代价。
    if (!renderer) buildScene()
    if (!renderer) return
    if (isShown.value && !isLeaving.value) {
      resize()
      return
    }
    isLeaving.value = false
    isShown.value = true
    revealStart = performance.now() / 1000
    energy.value = 0
    energy.phase = 0
    energy.lastPressMs = 0
    renderer?.setClearColor(0xffffff, 0.2)
    resize()
    playSound(props.openSound)
    startLoop()
    return
  }
  if (!isShown.value) {
    stopLoop()
    renderer?.setClearColor(0xffffff, 0)
    return
  }
  isLeaving.value = true
  leaveTimer = setTimeout(() => {
    isLeaving.value = false
    isShown.value = false
    stopLoop()
    renderer?.setClearColor(0xffffff, 0)
    bubbleText.value = ''
    lastMessage = ''
    leaveTimer = undefined
  }, LEAVE_SECONDS * 1000)
}

watch(
  () => props.active,
  (active) => {
    if (!active && !props.leaving) {
      setShown(false)
      return
    }
    setShown(active)
  },
)

watch(
  () => props.leaving,
  (leaving) => {
    if (leaving) setShown(false)
  },
)

watch(
  () => props.count,
  (value, previous) => {
    if (Number(value) !== Number(previous)) bump()
  },
)

watch(
  () => props.message,
  (value) => {
    const text = String(value ?? '').trim()
    if (!text) {
      bubbleText.value = ''
      lastMessage = ''
      return
    }
    if (text === lastMessage) return
    lastMessage = text
    bubbleText.value = text
    bubbleKey.value += 1
  },
)

watch(chainColor, (color) => {
  for (const chain of [chainA, chainB]) {
    const material = chain?.userData.mainMaterial as MeshPhysicalMaterial | undefined
    if (!material) continue
    material.color.copy(color)
    material.needsUpdate = true
  }
})

watch(
  () => [props.link, props.linkFine, props.blurMax],
  () => {
    if (!renderer) return
    if (post) post.material.uniforms.uBlurMax.value = Math.max(0, Number(props.blurMax) || 0)
    rebuildChains()
  },
)

onMounted(() => {
  resizeObserver = new ResizeObserver(() => resize())
  if (container.value) resizeObserver.observe(container.value)
  if (props.active && !props.leaving) setShown(true)
})

onBeforeUnmount(() => {
  stopLoop()
  if (bumpTimer) clearTimeout(bumpTimer)
  if (leaveTimer) clearTimeout(leaveTimer)
  resizeObserver?.disconnect()
  resizeObserver = null
  disposeChain(chainA)
  disposeChain(chainB)
  chainA = null
  chainB = null
  tubeGeometry?.dispose()
  tubeGeometry = null
  if (glow) {
    glow.geometry.dispose()
    const material = glow.material as MeshBasicMaterial
    material.map?.dispose()
    material.dispose()
    glow = null
  }
  if (post) {
    post.target.dispose()
    post.material.dispose()
    post = null
  }
  environment?.dispose()
  environment = null
  pmrem?.dispose()
  pmrem = null
  if (renderer) {
    renderer.dispose()
    renderer.domElement.remove()
    renderer = null
  }
  scene = null
  camera = null
  chainGroup = null
  frame = null
  audio = null
})

defineExpose({ triggerHit })
</script>

<template>
  <div ref="container" class="lock-chain-root" :class="rootClass">
    <div class="lock-chain-backdrop"><slot /></div>
    <div class="lock-chain-overlay">
      <div class="lock-chain-panel" :class="{ bump: isBumping }">
        <div class="lock-chain-title">{{ title }}</div>
        <div :key="bumpKey" class="lock-chain-number">{{ count }}</div>
        <div v-if="bubbleText" :key="bubbleKey" class="lock-chain-bubbles">
          <div class="lock-chain-bubble">{{ bubbleText }}</div>
        </div>
      </div>
    </div>
  </div>
</template>
