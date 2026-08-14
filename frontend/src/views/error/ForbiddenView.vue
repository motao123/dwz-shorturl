<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Lock } from '@element-plus/icons-vue'

const router = useRouter()
</script>

<template>
  <div class="forbidden">
    <div class="forbidden__icon"><el-icon :size="64"><Lock /></el-icon></div>
    <h1 class="forbidden__code">403</h1>
    <p class="forbidden__desc">你没有访问该页面的权限</p>
    <p class="forbidden__hint">如需访问，请联系管理员分配相应权限</p>
    <el-button type="primary" @click="router.push('/dashboard')">返回仪表盘</el-button>
  </div>
</template>

<style scoped>
.forbidden {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: var(--dwz-paper, #f2f5f7);
  text-align: center;
  padding: 24px;
}

.forbidden__icon {
  color: var(--dwz-amber-deep, #a8690a);
  margin-bottom: 8px;
}

.forbidden__code {
  font-family: var(--dwz-font-mono, 'JetBrains Mono', monospace);
  font-size: 56px;
  font-weight: 700;
  margin: 0;
  color: var(--dwz-ink, #16292b);
}

.forbidden__desc {
  font-size: 16px;
  font-weight: 600;
  margin: 4px 0 0;
  color: var(--dwz-ink, #16292b);
}

.forbidden__hint {
  font-size: 13px;
  margin: 0 0 20px;
  color: var(--dwz-text-dim, #6b7f86);
}
</style>
