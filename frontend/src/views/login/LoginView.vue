<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormRules, FormInstance } from 'element-plus'
import { User, Lock, Right } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  username: '',
  password: ''
})

const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 2, max: 32, message: '用户名长度为 2 - 32 个字符', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, max: 64, message: '密码长度至少 6 位', trigger: 'blur' }
  ]
}

async function handleLogin() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  loading.value = true
  try {
    await authStore.login({ username: form.username.trim(), password: form.password })
    ElMessage.success('登录成功，欢迎回来')
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    router.push(redirect.startsWith('/') ? redirect : '/dashboard')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '登录失败，请检查用户名或密码')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login">
    <!-- 品牌侧 -->
    <aside class="login__brand">
      <div class="login__glow login__glow--a"></div>
      <div class="login__glow login__glow--b"></div>

      <header class="brand-head">
        <div class="brand-head__mark">
          <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
            <path d="M10.5 13.5a3.5 3.5 0 0 1 0-5l2-2a3.54 3.54 0 0 1 5 5l-1.2 1.2"
              stroke="#f5a623" stroke-width="2.2" fill="none" stroke-linecap="round" />
            <path d="M13.5 10.5a3.5 3.5 0 0 1 0 5l-2 2a3.54 3.54 0 0 1-5-5l1.2-1.2"
              stroke="#e8f4f2" stroke-width="2.2" fill="none" stroke-linecap="round" />
          </svg>
        </div>
        <div>
          <div class="brand-head__name">DWZ 控制台</div>
          <div class="brand-head__sub mono">SHORT URL CONSOLE</div>
        </div>
      </header>

      <div class="brand-mid">
        <h1 class="brand-title">
          把<span class="brand-title__hl">长链接</span>，<br />
          收进一个短码里。
        </h1>
        <p class="brand-desc">
          创建、追踪与管理每一条短链 —— 点击趋势、来源分布、权限审计，尽在一块仪表盘。
        </p>

        <!-- 漂浮短链芯片 -->
        <div class="chips" aria-hidden="true">
          <span class="chip mono chip--1">dwz.cn/aB3x9 <i>2,341 次点击</i></span>
          <span class="chip mono chip--2">dwz.cn/Kf0Qz <i>1,102 次点击</i></span>
          <span class="chip mono chip--3">dwz.cn/7mP2d <i>864 次点击</i></span>
        </div>
      </div>

      <footer class="brand-foot mono">
        <span class="brand-foot__dot"></span>
        SYSTEM ONLINE · {{ new Date().getFullYear() }}
      </footer>
    </aside>

    <!-- 表单侧 -->
    <main class="login__panel">
      <div class="panel-inner">
        <p class="panel-kicker mono">ADMIN · SIGN IN</p>
        <h2 class="panel-title">登录管理后台</h2>
        <p class="panel-sub">使用管理员账号登录，进入短链运营控制台</p>

        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          size="large"
          class="panel-form"
          @submit.prevent="handleLogin"
        >
          <el-form-item prop="username">
            <el-input
              v-model="form.username"
              placeholder="用户名"
              :prefix-icon="User"
              clearable
              autocomplete="username"
            />
          </el-form-item>
          <el-form-item prop="password">
            <el-input
              v-model="form.password"
              type="password"
              placeholder="密码"
              show-password
              :prefix-icon="Lock"
              autocomplete="current-password"
              @keyup.enter="handleLogin"
            />
          </el-form-item>

          <el-button
            type="primary"
            class="submit"
            :loading="loading"
            @click="handleLogin"
          >
            登 录
            <el-icon v-if="!loading" class="submit__arrow"><Right /></el-icon>
          </el-button>
        </el-form>

        <p class="panel-foot mono">仅限授权管理员访问 · 所有操作将被审计记录</p>
      </div>
    </main>
  </div>
</template>

<style scoped>
.login {
  display: flex;
  min-height: 100vh;
}

/* ---------------- 品牌侧 ---------------- */

.login__brand {
  position: relative;
  width: 46%;
  min-width: 480px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 40px 52px;
  overflow: hidden;
  color: #dcebe9;
  background:
    radial-gradient(rgba(255, 255, 255, 0.045) 1px, transparent 1px) 0 0 / 26px 26px,
    linear-gradient(160deg, #10363e 0%, #0a2227 55%, #071a1e 100%);
}

.login__glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  pointer-events: none;
}

.login__glow--a {
  width: 420px;
  height: 420px;
  left: -140px;
  top: -120px;
  background: rgba(14, 110, 117, 0.5);
  animation: drift 12s ease-in-out infinite alternate;
}

.login__glow--b {
  width: 380px;
  height: 380px;
  right: -160px;
  bottom: -140px;
  background: rgba(245, 166, 35, 0.16);
  animation: drift 14s ease-in-out infinite alternate-reverse;
}

@keyframes drift {
  from { transform: translate(0, 0) scale(1); }
  to { transform: translate(40px, 30px) scale(1.15); }
}

.brand-head {
  position: relative;
  display: flex;
  align-items: center;
  gap: 13px;
}

.brand-head__mark {
  width: 42px;
  height: 42px;
  display: grid;
  place-items: center;
  border-radius: 11px;
  background: rgba(245, 166, 35, 0.12);
  border: 1px solid rgba(245, 166, 35, 0.3);
}

.brand-head__name {
  font-weight: 800;
  font-size: 17px;
  color: #fff;
  letter-spacing: 0.02em;
}

.brand-head__sub {
  font-size: 9.5px;
  letter-spacing: 0.3em;
  color: #5e848b;
}

.brand-mid {
  position: relative;
}

.brand-title {
  margin: 0 0 16px;
  font-size: 42px;
  line-height: 1.22;
  font-weight: 800;
  letter-spacing: -0.02em;
  color: #f2faf8;
}

.brand-title__hl {
  position: relative;
  color: var(--dwz-amber);
}

.brand-title__hl::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 4px;
  height: 8px;
  background: rgba(245, 166, 35, 0.22);
  z-index: -1;
  border-radius: 2px;
}

.brand-desc {
  margin: 0;
  max-width: 400px;
  font-size: 15px;
  line-height: 1.8;
  color: #8fb0b3;
}

/* 漂浮芯片 */
.chips {
  position: relative;
  height: 150px;
  margin-top: 34px;
}

.chip {
  position: absolute;
  display: inline-flex;
  align-items: baseline;
  gap: 10px;
  padding: 9px 15px;
  font-size: 13px;
  color: #dff3ef;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  backdrop-filter: blur(4px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
}

.chip i {
  font-style: normal;
  font-size: 10.5px;
  color: var(--dwz-amber);
  letter-spacing: 0.04em;
}

.chip--1 { left: 4%; top: 0; animation: float 5.2s ease-in-out infinite; }
.chip--2 { left: 34%; top: 52px; animation: float 6.4s ease-in-out 0.8s infinite; }
.chip--3 { left: 10%; top: 104px; animation: float 5.8s ease-in-out 1.5s infinite; }

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-9px); }
}

.brand-foot {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 10px;
  letter-spacing: 0.24em;
  color: #4f747b;
}

.brand-foot__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #2dd4a7;
  box-shadow: 0 0 8px #2dd4a7;
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

/* ---------------- 表单侧 ---------------- */

.login__panel {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #fbfcfc;
  padding: 24px;
}

.panel-inner {
  width: 100%;
  max-width: 392px;
  animation: rise 0.5s cubic-bezier(0.22, 1, 0.36, 1) both;
}

@keyframes rise {
  from { opacity: 0; transform: translateY(16px); }
  to { opacity: 1; transform: translateY(0); }
}

.panel-kicker {
  margin: 0 0 10px;
  font-size: 10.5px;
  letter-spacing: 0.3em;
  color: var(--dwz-amber-deep);
}

.panel-title {
  margin: 0;
  font-size: 30px;
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--dwz-ink);
}

.panel-sub {
  margin: 8px 0 34px;
  font-size: 13.5px;
  color: var(--dwz-text-dim);
}

.panel-form :deep(.el-input__wrapper) {
  padding: 4px 14px;
  border-radius: 10px;
  box-shadow: 0 0 0 1px var(--dwz-line) inset;
  transition: box-shadow 0.18s ease;
}

.panel-form :deep(.el-input__wrapper:hover) {
  box-shadow: 0 0 0 1px #9cc3c6 inset;
}

.panel-form :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 2px var(--dwz-petrol) inset, 0 4px 12px rgba(14, 110, 117, 0.14);
}

.panel-form :deep(.el-form-item) {
  margin-bottom: 22px;
}

.submit {
  width: 100%;
  height: 46px;
  font-size: 15px;
  font-weight: 800;
  letter-spacing: 0.35em;
  border-radius: 10px;
  margin-top: 6px;
}

.submit__arrow {
  margin-left: 2px;
  transition: transform 0.2s ease;
}

.submit:hover .submit__arrow {
  transform: translateX(4px);
}

.panel-foot {
  margin: 26px 0 0;
  text-align: center;
  font-size: 10px;
  letter-spacing: 0.12em;
  color: #9cb1b6;
}

/* 小屏：隐藏品牌侧 */
@media (max-width: 960px) {
  .login__brand {
    display: none;
  }
}
</style>
