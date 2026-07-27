<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Check } from '@element-plus/icons-vue'
import { getAllConfigs, batchUpdateConfigs, type ConfigItem } from '@/api/configs'

const loading = ref(false)
const saving = ref(false)
const configs = ref<ConfigItem[]>([])
/** 编辑缓冲：config_key -> 字符串值 */
const edits = ref<Record<string, string>>({})
/** 记录已被修改的 key */
const dirtyKeys = ref<Set<string>>(new Set())

const GROUP_LABELS: Record<string, string> = {
  site: '站点信息',
  url: '短链规则',
  security: '安全策略',
  rate: '限流策略',
  cache: '缓存配置',
  notify: '通知配置',
  storage: '存储配置',
  other: '其他'
}

interface ConfigGroup {
  key: string
  label: string
  items: ConfigItem[]
}

const groups = computed<ConfigGroup[]>(() => {
  const map = new Map<string, ConfigItem[]>()
  for (const c of configs.value) {
    const prefix = c.config_key.includes('.')
      ? c.config_key.split('.')[0]
      : c.config_key.includes('_')
        ? c.config_key.split('_')[0]
        : 'other'
    const arr = map.get(prefix) ?? []
    arr.push(c)
    map.set(prefix, arr)
  }
  const known = Object.keys(GROUP_LABELS).filter((k) => k !== 'other')
  const sortedKeys = [...map.keys()].sort((a, b) => {
    const ia = known.indexOf(a)
    const ib = known.indexOf(b)
    return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib)
  })
  return sortedKeys.map((k) => ({
    key: k,
    label: GROUP_LABELS[k] ?? k.toUpperCase(),
    items: map.get(k)!
  }))
})

const dirtyCount = computed(() => dirtyKeys.value.size)

async function loadConfigs() {
  loading.value = true
  try {
    const res = await getAllConfigs()
    configs.value = Array.isArray(res) ? res : ((res as unknown as { list: ConfigItem[] })?.list ?? [])
    const buf: Record<string, string> = {}
    for (const c of configs.value) buf[c.config_key] = c.config_value
    edits.value = buf
    dirtyKeys.value = new Set()
  } catch (err) {
    configs.value = []
    ElMessage.error(err instanceof Error ? err.message : '加载配置失败')
  } finally {
    loading.value = false
  }
}

function onEdit(key: string, value: string) {
  const origin = configs.value.find((c) => c.config_key === key)?.config_value
  const next = new Set(dirtyKeys.value)
  if (origin !== value) next.add(key)
  else next.delete(key)
  dirtyKeys.value = next
}

function isDirty(key: string): boolean {
  return dirtyKeys.value.has(key)
}

function validateValue(item: ConfigItem, value: string): string | null {
  if (item.value_type === 'int' && value !== '' && !/^-?\d+$/.test(value)) {
    return `「${item.config_key}」需要整数值`
  }
  if (item.value_type === 'bool' && !['true', 'false', '0', '1'].includes(value)) {
    return `「${item.config_key}」需要布尔值（true / false）`
  }
  if (item.value_type === 'json' && value.trim()) {
    try {
      JSON.parse(value)
    } catch {
      return `「${item.config_key}」不是合法 JSON`
    }
  }
  return null
}

async function handleSave() {
  if (!dirtyCount.value) {
    ElMessage.info('没有需要保存的修改')
    return
  }

  // 类型校验
  for (const key of dirtyKeys.value) {
    const item = configs.value.find((c) => c.config_key === key)
    if (!item) continue
    const err = validateValue(item, edits.value[key] ?? '')
    if (err) {
      ElMessage.error(err)
      return
    }
  }

  saving.value = true
  try {
    const items = [...dirtyKeys.value].map((key) => ({
      config_key: key,
      config_value: edits.value[key] ?? ''
    }))
    await batchUpdateConfigs(items)
    ElMessage.success(`已保存 ${items.length} 项配置`)
    loadConfigs()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '保存配置失败')
  } finally {
    saving.value = false
  }
}

function resetAll() {
  const buf: Record<string, string> = {}
  for (const c of configs.value) buf[c.config_key] = c.config_value
  edits.value = buf
  dirtyKeys.value = new Set()
  ElMessage.info('已还原未保存的修改')
}

onMounted(loadConfigs)
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          系统配置
          <small>CONFIGS · 全局参数</small>
        </h1>
        <p class="app-page__desc">修改后点击「保存全部」生效，带 <span class="dirty-dot"></span> 标记的为未保存项</p>
      </div>
      <div class="head-actions">
        <el-button :icon="Refresh" @click="loadConfigs">刷新</el-button>
        <el-button :disabled="!dirtyCount" @click="resetAll">还原修改</el-button>
        <el-button type="primary" :icon="Check" :loading="saving" :disabled="!dirtyCount" @click="handleSave">
          保存全部<span v-if="dirtyCount" class="mono">&nbsp;({{ dirtyCount }})</span>
        </el-button>
      </div>
    </div>

    <div v-loading="loading" class="config-groups">
      <el-empty v-if="!loading && !configs.length" description="暂无配置项" />

      <section v-for="group in groups" :key="group.key" class="config-group app-card">
        <header class="config-group__head">
          <h3 class="config-group__title">{{ group.label }}</h3>
          <span class="config-group__prefix mono">{{ group.key }}.*</span>
        </header>

        <div class="config-rows">
          <div
            v-for="item in group.items"
            :key="item.config_key"
            class="config-row"
            :class="{ 'config-row--dirty': isDirty(item.config_key) }"
          >
            <div class="config-row__meta">
              <code class="config-row__key mono">{{ item.config_key }}</code>
              <span class="config-row__type mono">{{ item.value_type }}</span>
              <p class="config-row__desc">{{ item.description || '—' }}</p>
            </div>
            <div class="config-row__input">
              <template v-if="item.value_type === 'bool'">
                <el-switch
                  :model-value="['true', '1'].includes(edits[item.config_key] ?? '')"
                  @change="(v: string | number | boolean) => { edits[item.config_key] = String(v); onEdit(item.config_key, String(v)) }"
                />
              </template>
              <template v-else-if="item.value_type === 'json'">
                <el-input
                  v-model="edits[item.config_key]"
                  type="textarea"
                  :rows="3"
                  class="mono"
                  placeholder="JSON"
                  @input="(v: string) => onEdit(item.config_key, v)"
                />
              </template>
              <template v-else>
                <el-input
                  v-model="edits[item.config_key]"
                  :class="{ mono: item.value_type === 'int' }"
                  @input="(v: string) => onEdit(item.config_key, v)"
                >
                  <template v-if="item.is_public === 1" #suffix>
                    <el-tag size="small" effect="plain" round>公开</el-tag>
                  </template>
                </el-input>
              </template>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.head-actions {
  display: flex;
  gap: 10px;
}

.dirty-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--dwz-amber);
  vertical-align: 1px;
}

.config-groups {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.config-group {
  overflow: hidden;
}

.config-group__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 15px 20px;
  background: linear-gradient(90deg, #f4f8f8, #ffffff);
  border-bottom: 1px solid var(--dwz-line);
}

.config-group__title {
  margin: 0;
  font-size: 15px;
  font-weight: 800;
  color: var(--dwz-ink);
}

.config-group__prefix {
  font-size: 11px;
  color: var(--dwz-text-dim);
  letter-spacing: 0.06em;
}

.config-row {
  display: grid;
  grid-template-columns: minmax(240px, 380px) 1fr;
  gap: 24px;
  align-items: center;
  padding: 14px 20px;
  border-bottom: 1px solid #eef3f4;
  transition: background-color 0.15s ease;
  position: relative;
}

.config-row:last-child {
  border-bottom: none;
}

.config-row:hover {
  background: #f8fbfb;
}

.config-row--dirty::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: var(--dwz-amber);
}

.config-row__meta {
  min-width: 0;
}

.config-row__key {
  font-size: 13px;
  font-weight: 600;
  color: var(--dwz-petrol-strong);
  word-break: break-all;
}

.config-row__type {
  margin-left: 8px;
  padding: 1px 7px;
  border-radius: 5px;
  background: #eef3f4;
  color: var(--dwz-text-dim);
  font-size: 10px;
  letter-spacing: 0.05em;
}

.config-row__desc {
  margin: 5px 0 0;
  font-size: 12px;
  color: var(--dwz-text-dim);
  line-height: 1.5;
}

.config-row__input {
  min-width: 0;
}

.config-row__input :deep(.el-textarea__inner),
.config-row__input :deep(.el-input__wrapper) {
  font-size: 13px;
}

@media (max-width: 900px) {
  .config-row {
    grid-template-columns: 1fr;
    gap: 10px;
  }
}
</style>
