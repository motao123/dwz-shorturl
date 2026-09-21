<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormRules, FormInstance } from 'element-plus'
import { Sunny, Moon } from '@element-plus/icons-vue'
import { useThemeStore } from '@/stores/theme'
import { resolveSiteRedirect } from '@/utils/redirect'
import { login, register, requestPasswordReset, sendVerification } from '../api'

const router = useRouter()
const route = useRoute()
const themeStore = useThemeStore()

// 登录成功后跳回 redirect 参数指定的页面（仅允许站内路径），否则进入会员中心。
// 解析与开放重定向防护见 utils/redirect.ts：除根路径外也接受相对值，因为静态首页在
// 子目录部署（/short/）里拼不出正确的根绝对路径（#48）。本 SPA 固定挂在
// <root>/member/ 下，所以站点根就是 baseURI 的上两级。
const siteRoot = new URL('..', document.baseURI).href
const redirectTo = computed(() => resolveSiteRedirect(route.query.redirect, siteRoot, '/'))

const loading = ref(false)
const mode = ref<'login' | 'register' | 'forgot'>('login')
const formRef = ref<FormInstance>()
const forgotEmail = ref('')
const forgotLoading = ref(false)

const form = reactive({ username: '', email: '', password: '', agreed: false })

const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 2, max: 32, message: '用户名 2-32 位', trigger: 'blur' }
  ],
  email: [{ required: true, message: '请输入邮箱', trigger: 'blur' }],
  agreed: [
    {
      validator: (_r, v, cb) => {
        if (mode.value !== 'register') return cb()
        if (!v) return cb(new Error('请先阅读并同意《用户协议》与《隐私政策》'))
        cb()
      },
      trigger: 'change'
    }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 8, max: 64, message: '密码 8-64 位', trigger: 'blur' },
    {
      validator: (_r, v, cb) => {
        if (mode.value === 'login' || !v) return cb()
        if (!/[A-Za-z]/.test(v) || !/[0-9]/.test(v)) return cb(new Error('密码需同时包含字母和数字'))
        cb()
      },
      trigger: 'blur'
    }
  ]
}

async function submit() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  loading.value = true
  try {
    if (mode.value === 'login') {
      await login(form.username.trim(), form.password)
      ElMessage.success('登录成功')
    } else {
      await register(form.username.trim(), form.email.trim(), form.password)
      // 注册后自动发送邮箱验证邮件（失败不阻断注册，但必须如实告知用户）。
      // 原先无条件提示「验证邮件已发送」会让未配置 SMTP 的站点用户一直空等。
      let verifySent = true
      try {
        await sendVerification(form.email.trim())
      } catch {
        verifySent = false
      }
      if (verifySent) {
        ElMessage.success('注册成功，验证邮件已发送至您的邮箱（24 小时内有效），验证后可解锁批量生成')
      } else {
        ElMessage.warning('注册成功，但验证邮件发送失败：请到会员中心「重新发送验证邮件」，或联系管理员配置邮件服务')
      }
    }
    if (redirectTo.value !== '/') {
      // 有 redirect 参数时整页跳转（如返回首页批量区）
      window.location.href = redirectTo.value
    } else {
      router.push('/')
    }
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '操作失败')
  } finally {
    loading.value = false
  }
}

async function handleForgot() {
  if (!forgotEmail.value.trim()) {
    ElMessage.warning('请输入注册邮箱')
    return
  }
  forgotLoading.value = true
  try {
    await requestPasswordReset(forgotEmail.value.trim())
    ElMessage.success('重置邮件已发送，请查收邮箱（30 分钟内有效）')
    mode.value = 'login'
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '发送失败')
  } finally {
    forgotLoading.value = false
  }
}
</script>

<template>
  <div class="login-wrap member-login">
    <button class="theme-toggle" :aria-label="themeStore.dark ? '切换到浅色主题' : '切换到深色主题'" :title="themeStore.dark ? '切换到浅色' : '切换到深色'" @click="themeStore.toggle()">
      <el-icon :size="17"><component :is="themeStore.dark ? Sunny : Moon" /></el-icon>
    </button>
    <div class="login-card">
      <h1 class="brand">短网址 <span>· 会员中心</span></h1>
      <el-tabs v-model="mode" stretch>
        <el-tab-pane label="登录" name="login" />
        <el-tab-pane label="注册" name="register" />
        <el-tab-pane label="忘记密码" name="forgot" />
      </el-tabs>

      <!-- 忘记密码 -->
      <el-form v-if="mode === 'forgot'" @submit.prevent="handleForgot">
        <el-form-item>
          <el-input v-model="forgotEmail" placeholder="注册邮箱" size="large" @keyup.enter="handleForgot" />
        </el-form-item>
        <el-button type="primary" size="large" class="submit" :loading="forgotLoading" @click="handleForgot">
          发送重置邮件
        </el-button>
      </el-form>

      <el-form v-else ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名" size="large" />
        </el-form-item>
        <el-form-item v-if="mode === 'register'" prop="email">
          <el-input v-model="form.email" placeholder="邮箱" size="large" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="密码" size="large" @keyup.enter="submit" />
        </el-form-item>
        <el-form-item v-if="mode === 'register'" prop="agreed">
          <el-checkbox v-model="form.agreed">
            我已阅读并同意
            <a href="/terms.html" target="_blank" rel="noopener">《用户协议》</a>与
            <a href="/privacy.html" target="_blank" rel="noopener">《隐私政策》</a>
          </el-checkbox>
        </el-form-item>
        <el-button type="primary" size="large" class="submit" :loading="loading" @click="submit">
          {{ mode === 'login' ? '登 录' : '注 册' }}
        </el-button>
      </el-form>

      <p class="foot"><a href="/">← 返回首页</a></p>
    </div>
  </div>
</template>

<style scoped>
/* 与管理台共用一套墨青主题 token（--dwz-petrol），不再使用与其他端割裂的
   深青渐变，避免会员从首页跳到登录页时「像换了一个站点」。 */
.login-wrap {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background:
    radial-gradient(1200px 600px at 50% -10%, color-mix(in srgb, var(--dwz-petrol, #0e6e75) 10%, transparent), transparent 70%),
    var(--el-bg-color-page, #f5f7f8);
  padding: 20px;
}

.theme-toggle {
  position: fixed;
  top: 18px;
  right: 18px;
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border: 1px solid rgba(255, 255, 255, 0.25);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.08);
  color: #fff;
  cursor: pointer;
  transition: background 0.15s ease;
}

.theme-toggle:hover {
  background: rgba(255, 255, 255, 0.18);
}

.login-card {
  width: 380px;
  padding: 32px;
  background: #fff;
  border-radius: 14px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.3);
}

.brand {
  margin: 0 0 20px;
  font-size: 22px;
  color: #0e6e75;
  text-align: center;
}

.brand span {
  font-size: 13px;
  color: #9aa7aa;
  font-weight: 400;
}

.submit {
  width: 100%;
  margin-top: 6px;
}

.foot {
  margin: 18px 0 0;
  text-align: center;
  font-size: 13px;
}

.foot a {
  color: #0e6e75;
  text-decoration: none;
}
</style>