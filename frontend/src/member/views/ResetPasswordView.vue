<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormRules, FormInstance } from 'element-plus'
import { Sunny, Moon } from '@element-plus/icons-vue'
import { useThemeStore } from '@/stores/theme'
import { resetPassword } from '../api'

const route = useRoute()
const router = useRouter()
const themeStore = useThemeStore()

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({ password: '', confirm: '' })

const rules: FormRules = {
  password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 8, max: 64, message: '密码 8-64 位', trigger: 'blur' },
    {
      validator: (_r, v, cb) => {
        if (!v) return cb()
        if (!/[A-Za-z]/.test(v) || !/[0-9]/.test(v)) return cb(new Error('密码需同时包含字母和数字'))
        cb()
      },
      trigger: 'blur'
    }
  ],
  confirm: [
    {
      validator: (_r, v, cb) => {
        if (!v) return cb(new Error('请再次输入新密码'))
        if (v !== form.password) return cb(new Error('两次输入的密码不一致'))
        cb()
      },
      trigger: 'blur'
    }
  ]
}

async function submit() {
  if (!token.value) {
    ElMessage.error('重置链接无效')
    return
  }
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  loading.value = true
  try {
    await resetPassword(token.value, form.password)
    ElMessage.success('密码已重置，请用新密码登录')
    router.push('/login')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '重置失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap member-login">
    <button class="theme-toggle" :title="themeStore.dark ? '切换到浅色' : '切换到深色'" @click="themeStore.toggle()">
      <el-icon :size="17"><component :is="themeStore.dark ? Sunny : Moon" /></el-icon>
    </button>
    <div class="login-card">
      <h1 class="brand">短网址 <span>会员中心</span></h1>
      <h2 class="reset-title">设置新密码</h2>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
        <el-form-item prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="新密码（8-64 位，含字母和数字）" size="large" />
        </el-form-item>
        <el-form-item prop="confirm">
          <el-input v-model="form.confirm" type="password" show-password placeholder="确认新密码" size="large" @keyup.enter="submit" />
        </el-form-item>
        <el-button type="primary" size="large" class="submit" :loading="loading" @click="submit">
          重置密码
        </el-button>
      </el-form>
      <p class="foot"><a href="/">← 返回首页</a></p>
    </div>
  </div>
</template>

<style scoped>
.login-wrap {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background: linear-gradient(160deg, #10363e, #0a2227);
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
  margin: 0 0 6px;
  font-size: 22px;
  color: #0e6e75;
  text-align: center;
}

.brand span {
  font-size: 13px;
  color: #9aa7aa;
}

.reset-title {
  margin: 0 0 18px;
  font-size: 15px;
  color: #444;
  text-align: center;
  font-weight: 600;
}

.submit {
  width: 100%;
}

.foot {
  margin: 16px 0 0;
  text-align: center;
  font-size: 13px;
}

.foot a {
  color: #0e6e75;
  text-decoration: none;
}
</style>