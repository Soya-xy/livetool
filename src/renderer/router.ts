import { createRouter, createWebHashHistory } from 'vue-router'
import ControlCenter from './pages/ControlCenter.vue'
import DanmakuDiary from './pages/DanmakuDiary.vue'
import Extensions from './pages/Extensions.vue'
import Settings from './pages/Settings.vue'
import LicenseAdmin from './pages/LicenseAdmin.vue'
import OverlayGreen from './pages/OverlayGreen.vue'
import OverlaySlot from './pages/OverlaySlot.vue'
import AudioOverlay from './pages/AudioOverlay.vue'

export default createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/control' },
    { path: '/control', component: ControlCenter },
    { path: '/diary', component: DanmakuDiary },
    { path: '/extensions', component: Extensions },
    { path: '/settings', component: Settings },
    { path: '/license-admin', component: LicenseAdmin },
    { path: '/overlay-green', component: OverlayGreen },
    { path: '/overlay-slot', component: OverlaySlot },
    { path: '/overlay-audio', component: AudioOverlay },
  ],
})
