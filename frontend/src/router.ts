import { createRouter, createWebHashHistory } from 'vue-router'

export default createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/control' },
    { path: '/control', component: () => import('./pages/ControlCenter.vue') },
    { path: '/diary', component: () => import('./pages/DanmakuDiary.vue') },
    { path: '/extensions', component: () => import('./pages/Extensions.vue') },
    { path: '/settings', component: () => import('./pages/Settings.vue') },
    { path: '/overlay-green', component: () => import('./pages/OverlayGreen.vue') },
    { path: '/overlay-slot', component: () => import('./pages/OverlaySlot.vue') },
    { path: '/overlay-audio', component: () => import('./pages/AudioOverlay.vue') },
  ],
})
