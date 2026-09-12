<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { Refresh, Check, Operation } from '@element-plus/icons-vue'
import { getAllConfigs, batchUpdateConfigs, type ConfigItem } from '@/api/configs'
import { runTask } from '@/api/monitor'

const loading = ref(false)
const saving = ref(false)
const configs = ref<ConfigItem[]>([])
/** 编辑缓冲：config_key -> 字符串值 */
const edits = ref<Record<string, string>>({})
/** 记录已被修改的 key */
const dirtyKeys = ref<Set<string>>(new Set())

const GROUP_LABELS: Record<string, string> = {
  shorturl: '短链规则',
  batch: '批量生成',
  member: '会员策略',
  stats: '统计页面',
  site: '站点信息',
  url: '短链规则',
  security: '安全策略',
  rate: '限流策略',
  cache: '缓存配置',
  notify: '通知配置',
  storage: '存储配置',
  other: '其他'
}

/** 取值有限的配置项渲染为下拉框（值 -> 展示文案） */
const ENUM_OPTIONS: Record<string, { value: string; label: string }[]> = {
  'shorturl.default_expire_days': [
    { value: '0', label: '永久有效' },
    { value: '1', label: '1 天' },
    { value: '7', label: '7 天' },
    { value: '30', label: '30 天' },
    { value: '365', label: '1 年' }
  ]
}

/** 运维操作：手动触发后端 cron 任务（与「系统监控」页同一套任务） */
const OPS_TASKS: { name: string; label: string; hint: string; danger?: boolean }[] = [
  { name: 'mark_expired', label: '标记过期短链', hint: '扫描到期链接并置为过期状态' },
  { name: 'reconcile_dual_write', label: '双写对账', hint: '合并 PHP 前台与后台的短链数据' },
  { name: 'reconcile_clicks', label: '点击数校准', hint: '双向校准两表的点击计数' },
  { name: 'aggregate_stats', label: '统计聚合', hint: '把点击明细聚合到小时表，加速报表' },
  { name: 'cleanup_click_logs', label: '清理过期点击日志', hint: '删除超出保留期的点击明细', danger: true },
  { name: 'cleanup_stats', label: '清理历史统计', hint: '删除过期的聚合统计数据', danger: true },
  { name: 'remind_expiring', label: '发送到期提醒', hint: '给临期链接的属主发送提醒（需邮件服务）' },
  { name: 'ensure_partitions', label: '补齐日志分区', hint: '创建缺失的 click_logs 月度分区（幂等）' }
]
const runningTask = ref('')

async function handleRunTask(task: (typeof OPS_TASKS)[number]) {
  try {
    await ElMessageBox.confirm(
      `确定立即执行「${task.label}」吗？${task.hint}。该操作同步执行，大数据量时可能耗时较长。`,
      '运维操作确认',
      { type: task.danger ? 'warning' : 'info', confirmButtonText: '执行', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  runningTask.value = task.name
  try {
    const res = await runTask(task.name)
    ElMessage.success(res.ran ? `「${task.label}」执行完成` : `「${task.label}」本次无需处理`)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '任务执行失败')
  } finally {
    runningTask.value = ''
  }
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
        <el-button :icon="Refresh" aria-label="重新加载系统配置" @click="loadConfigs">刷新</el-button>
        <el-button :disabled="!dirtyCount" aria-label="放弃未保存的系统配置修改" @click="resetAll">
          还原修改
        </el-button>
        <el-button
          type="primary"
          :icon="Check"
          :loading="saving"
          :disabled="!dirtyCount"
          aria-label="保存全部系统配置修改"
          @click="handleSave"
        >
          保存全部<span v-if="dirtyCount" class="mono">&nbsp;({{ dirtyCount }})</span>
        </el-button>
      </div>
    </div>

    <el-tabs class="config-tabs">
      <el-tab-pane label="参数配置">
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
                  <template v-if="ENUM_OPTIONS[item.config_key]">
                    <el-select
                      :model-value="edits[item.config_key]"
                      @change="(v: unknown) => { edits[item.config_key] = String(v); onEdit(item.config_key, String(v)) }"
                    >
                      <el-option
                        v-for="o in ENUM_OPTIONS[item.config_key]"
                        :key="o.value"
                        :value="o.value"
                        :label="o.label"
                      />
                    </el-select>
                  </template>
                  <template v-else-if="item.value_type === 'bool'">
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
      </el-tab-pane>

      <el-tab-pane label="运维操作">
        <p class="ops-hint">
          以下操作与「系统监控」页的定时任务同源，点击后<strong>立即同步执行</strong>一次；
          带 <span class="ops-danger-tag">!</span> 的操作会删除历史数据，请确认后执行。
        </p>
        <div class="ops-grid">
          <div v-for="task in OPS_TASKS" :key="task.name" class="ops-card app-card" :class="{ 'ops-card--danger': task.danger }">
            <div class="ops-card__head">
              <el-icon class="ops-card__icon"><Operation /></el-icon>
              <strong>{{ task.label }}</strong>
              <code class="mono ops-card__name">{{ task.name }}</code>
            </div>
            <p class="ops-card__hint">{{ task.hint }}</p>
            <el-button
              size="small"
              :type="task.danger ? 'danger' : 'primary'"
              plain
              :loading="runningTask === task.name"
              @click="handleRunTask(task)"
            >
              立即执行
            </el-button>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>
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

.config-tabs {
  margin-top: 4px;
}

.ops-hint {
  margin: 0 0 14px;
  font-size: 13px;
  color: var(--dwz-text-dim);
}

.ops-danger-tag {
  display: inline-block;
  width: 14px;
  height: 14px;
  line-height: 14px;
  text-align: center;
  border-radius: 50%;
  background: var(--dwz-red, #d54941);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
}

.ops-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 14px;
}

.ops-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px 18px;
}

.ops-card--danger {
  border-color: rgba(213, 73, 65, 0.35);
}

.ops-card__head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: var(--dwz-ink);
}

.ops-card__icon {
  color: var(--dwz-petrol-strong);
}

.ops-card__name {
  margin-left: auto;
  font-size: 10px;
  color: var(--dwz-text-dim);
}

.ops-card__hint {
  margin: 0;
  flex: 1;
  font-size: 12px;
  line-height: 1.5;
  color: var(--dwz-text-dim);
}
</style>
