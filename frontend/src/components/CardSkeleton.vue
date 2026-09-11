<script setup lang="ts">
/**
 * 卡片骨架屏
 *
 * 用于卡片/键值对类区块（监控页、配置页等）的首屏加载。
 * 通过 `lines` 控制占位行数，高度与真实 `.kv` 行一致以保证无跳动。
 */
withDefaults(
  defineProps<{
    /** 标题占位宽度 */
    titleWidth?: string
    /** 键值行数 */
    lines?: number
    /** 是否显示图标位 */
    icon?: boolean
  }>(),
  {
    titleWidth: '96px',
    lines: 3,
    icon: true
  }
)
</script>

<template>
  <div class="sk-card" role="status" aria-live="polite" aria-busy="true" aria-label="卡片加载中">
    <span class="sk-sr">加载中…</span>
    <div class="sk-card__title" aria-hidden="true">
      <span v-if="icon" class="sk-block sk-block--icon" />
      <span class="sk-block sk-block--title" :style="{ width: titleWidth }" />
    </div>
    <div v-for="i in lines" :key="i" class="sk-card__row" aria-hidden="true">
      <span class="sk-block sk-block--key" />
      <span class="sk-block sk-block--val" />
    </div>
  </div>
</template>

<style scoped>
.sk-sr {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
}

.sk-card {
  position: relative;
}

.sk-card__title {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 21px;
  margin: 0 0 14px;
}

.sk-card__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 36px;
  border-bottom: 1px solid var(--dwz-line);
}

.sk-card__row:last-child {
  border-bottom: none;
}

.sk-block {
  display: block;
  border-radius: 4px;
  background: var(--dwz-line);
}

.sk-block--icon {
  width: 16px;
  height: 16px;
  flex: none;
}

.sk-block--title {
  height: 14px;
}

.sk-block--key {
  width: 64px;
  height: 12px;
}

.sk-block--val {
  width: 88px;
  height: 12px;
}
</style>
