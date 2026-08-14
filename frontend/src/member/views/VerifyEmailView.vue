<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { verifyEmail } from '../api'

const route = useRoute()
const router = useRouter()
const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
const status = ref<'loading' | 'ok' | 'fail'>('loading')
const msg = ref('正在验证邮箱…')

onMounted(async () => {
  if (!token.value) {
    status.value = 'fail'
    msg.value = '验证链接无效'
    return
  }
  try {
    await verifyEmail(token.value)
    status.value = 'ok'
    msg.value = '邮箱验证成功！'
  } catch (err) {
    status.value = 'fail'
    msg.value = err instanceof Error ? err.message : '验证失败'
  }
})
</script>

<template>
  <div class="verify-wrap">
    <div class="verify-card">
      <div class="mark" :class="status">{{ status === 'ok' ? '✓' : status === 'fail' ? '✕' : '…' }}</div>
      <h1>{{ msg }}</h1>
      <p v-if="status === 'ok'" class="sub">现在可以正常使用会员中心全部功能。</p>
      <el-button type="primary" size="large" @click="router.push('/login')">去登录</el-button>
    </div>
  </div>
</template>

<style scoped>
.verify-wrap {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background: linear-gradient(160deg, #10363e, #0a2227);
  padding: 20px;
}

.verify-card {
  width: 360px;
  padding: 36px;
  background: #fff;
  border-radius: 14px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.3);
  text-align: center;
}

.mark {
  width: 56px;
  height: 56px;
  margin: 0 auto 16px;
  display: grid;
  place-items: center;
  border-radius: 50%;
  font-size: 26px;
  color: #fff;
}

.mark.ok {
  background: #16a34a;
}

.mark.fail {
  background: #dc2626;
}

.mark.loading {
  background: #f5a623;
}

h1 {
  margin: 0 0 8px;
  font-size: 18px;
  color: #222;
}

.sub {
  margin: 0 0 20px;
  color: #888;
  font-size: 13px;
}
</style>