import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { ElMessage } from 'element-plus'
import 'element-plus/theme-chalk/el-message.css'
import 'element-plus/theme-chalk/el-message-box.css'
import App from './App.vue'
import router from './router'
import './styles.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
// 兜底：任何没被页面级错误边界拦下的异常都留痕并提示，避免出现无反馈的空白界面。
app.config.errorHandler = (error, _instance, info) => {
  console.error(`[renderer] ${info}`, error)
  ElMessage.error(error instanceof Error ? error.message : '界面出现未处理的错误')
}
app.mount('#app')
