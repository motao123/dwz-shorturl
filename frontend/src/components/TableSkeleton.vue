<script setup lang="ts">
/**
 * 表格骨架屏
 *
 * 用于表格类列表的**首屏加载**：以等宽/等高的占位块替代数据行，
 * 保证数据到达前后布局尺寸一致，避免整页遮罩 + CLS 跳动。
 *
 * 列宽通过 `widths` 传入（与真实列 width/min-width 对齐），
 * 行数默认取当前分页 page-size。
 */
withDefaults(
  defineProps<{
    /** 骨架行数，建议等于当前分页 page-size */
    rows?: number
    /** 每列宽度，支持 number（px）或 string（任意 CSS 长度），如 ['44', '160', 'minmax(240px,1fr)'] */
    widths?: (number | string)[]
    /** 是否渲染表头占位 */
    header?: boolean
  }>(),
  {
    rows: 8,
    header: true,
    widths: () => ['60px', 'minmax(140px, 1fr)', 'minmax(240px, 2fr)', '90px', '90px', '120px']
  }
)

function toSize(w: number | string): string {
  return typeof w === 'number' ? `${w}px` : w
}
</script>

<template>
  <div class="sk-table" role="status" aria-live="polite" aria-busy="true" aria-label="表格加载中">
    <span class="sk-sr">加载中…</span>

    <div v-if="header" class="sk-table__head" aria-hidden="true">
      <span v-for="(w, i) in widths" :key="`h-${i}`" class="sk-block sk-block--head" :style="{ width: toSize(w) }" />
    </div>

    <div
      v-for="r in rows"
      :key="`r-${r}`"
      class="sk-table__row"
      :class="{ 'sk-table__row--alt': r % 2 === 0 }"
      aria-hidden="true"
    >
      <span
        v-for="(w, i) in widths"
        :key="`c-${r}-${i}`"
        class="sk-block"
        :style="{ width: toSize(w) }"
      />
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

.sk-table {
  position: relative;
  padding: 4px 0;
}

.sk-table__head,
.sk-table__row {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 12px;
}

.sk-table__head {
  height: 40px;
  border-bottom: 1px solid var(--dwz-line);
}

.sk-table__row {
  height: 48px;
  border-bottom: 1px solid var(--dwz-line);
}

.sk-table__row--alt {
  background: var(--el-fill-color-lighter, #fafcfc);
}

.sk-table__row:last-child {
  border-bottom: none;
}

/* 占位块尺寸按真实行高/文字高度设定，避免数据到达时跳动 */
.sk-block {
  display: block;
  height: 14px;
  border-radius: 4px;
  background: var(--dwz-line);
  max-width: 100%;
}

.sk-block--head {
  height: 11px;
  opacity: 0.85;
}
</style>
