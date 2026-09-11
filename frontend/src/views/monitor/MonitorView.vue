<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, computed } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { Refresh, Cpu, Connection, DataBoard, Timer, Odometer } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { getMonitorStatus, type MonitorStatus } from '@/api/monitor'
import CardSkeleton from '@/components/CardSkeleton.vue'
import TableSkeleton from '@/components/TableSkeleton.vue'

/** 首屏加载：仅此时展示骨架屏 */
const loading = ref(false)
/** 静默刷新：轮询/手动刷新时不遮罩，仅显示局部指示 */
const refreshing = ref(false)
/** 首次加载是否已完成（首屏骨架 → 内容的切换开关） */
const initialized = ref(false)
const status = ref<MonitorStatus | null>(null)

const CRON_LABELS: Record<string, string> = {
  mark_expired: '过期链接标记',
  cleanup_click_logs: '点击日志清理',
  aggregate_stats: '统计预聚合',
  cleanup_stats: '统计缓存清理',
  remind_expiring: '到期邮件提醒',
  reconcile_dual_write: '双写对账',
  reconcile_clicks: '点击数对账',
  ensure_partitions: '分区维护'
}

/** 定时任务表骨架列宽，与真实列（任务自适应 / 上次运行 200px）对齐 */
const cronSkeletonWidths = ['minmax(120px, 1fr)', '200px']

const REFRESH_INTERVAL = 30_000
let timer: ReturnType<typeof setInterval> | null = null
/** 防止慢请求叠加：同一时刻只允许一个在途请求 */
let inFlight: Promise<void> | null = null

const isFirstLoad = computed(() => loading.value && !initialized.value)

/**
 * 拉取监控数据。
 * @param silent 静默模式（轮询）：不显示骨架/遮罩，只在顶部显示 2px 进度条
 */
async function load(silent = false) {
  if (inFlight) return inFlight

  if (silent) {
    refreshing.value = true
  } else {
    loading.value = true
  }

  inFlight = (async () => {
    try {
      status.value = await getMonitorStatus()
      initialized.value = true
    } catch (err) {
      // 轮询失败保持上一次数据，避免整页闪空
      if (!silent) {
        ElMessage.error(err instanceof Error ? err.message : '加载监控数据失败')
      }
    } finally {
      loading.value = false
      refreshing.value = false
      inFlight = null
    }
  })()

  return inFlight
}

/** 手动刷新按钮：已有数据时走静默刷新，避免整页遮罩 */
function handleRefresh() {
  load(initialized.value)
}

function fmtTime(iso: string | null): string {
  if (!iso) return '从未运行'
  return dayjs(iso).format('YYYY-MM-DD HH:mm:ss')
}

// 30s 自动刷新（静默），组件卸载时清理定时器与在途标记
onMounted(() => {
  load()
  timer = setInterval(() => load(true), REFRESH_INTERVAL)
})
onBeforeUnmount(() => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  inFlight = null
})
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          系统监控
          <small>MONITOR · 服务健康与后台任务</small>
        </h1>
        <p class="app-page__desc">数据库 · Redis · 点击队列 · 定时任务</p>
      </div>
      <el-button
        type="primary"
        :icon="Refresh"
        :loading="loading"
        aria-label="刷新系统监控数据"
        @click="handleRefresh"
      >
        刷新<span v-if="refreshing" class="dwz-silent__dot" aria-hidden="true" />
      </el-button>
    </div>

    <!-- 首屏骨架：与真实卡片结构（标题 + 3/4/1 行键值 + 表格）同尺寸 -->
    <div v-if="isFirstLoad" class="monitor-grid" role="status" aria-busy="true" aria-label="监控数据加载中">
      <section class="app-card"><CardSkeleton :lines="3" title-width="84px" /></section>
      <section class="app-card"><CardSkeleton :lines="4" title-width="72px" /></section>
      <section class="app-card"><CardSkeleton :lines="1" title-width="60px" /></section>
      <section class="app-card"><CardSkeleton :lines="1" title-width="84px" /></section>
      <section class="app-card wide">
        <TableSkeleton header :rows="8" :widths="cronSkeletonWidths" />
      </section>
    </div>

    <div
      v-else-if="status"
      class="monitor-grid dwz-silent"
      :class="{ 'is-refreshing': refreshing }"
      :aria-busy="refreshing"
    >
      <!-- 静默刷新指示：顶部 2px 进度条，不遮罩内容、不阻塞交互 -->
      <span v-if="refreshing" class="dwz-silent__bar" aria-hidden="true" />

      <!-- 系统信息 -->
      <section class="app-card">
        <h3 class="card-title"><el-icon><Cpu /></el-icon>系统信息</h3>
        <dl class="kv">
          <div><dt>运行时长</dt><dd class="mono dwz-silent-value" :class="{ 'is-refreshing': refreshing }">{{ status.uptime }}</dd></div>
          <div><dt>启动时间</dt><dd class="mono">{{ fmtTime(status.start_time) }}</dd></div>
          <div><dt>Goroutine 数</dt><dd class="mono dwz-silent-value" :class="{ 'is-refreshing': refreshing }">{{ status.goroutines }}</dd></div>
        </dl>
      </section>

      <!-- 数据库 -->
      <section class="app-card">
        <h3 class="card-title">
          <el-icon><DataBoard /></el-icon>数据库
          <el-tag
            :type="status.db?.healthy ? 'success' : 'danger'"
            size="small"
            round
            :aria-label="`数据库状态：${status.db?.healthy ? '正常' : '异常'}`"
          >
            {{ status.db?.healthy ? '正常' : '异常' }}
          </el-tag>
        </h3>
        <dl class="kv">
          <div><dt>连接数</dt><dd class="mono dwz-silent-value" :class="{ 'is-refreshing': refreshing }">{{ status.db?.open_conns ?? 0 }}</dd></div>
          <div><dt>使用中</dt><dd class="mono dwz-silent-value" :class="{ 'is-refreshing': refreshing }">{{ status.db?.in_use ?? 0 }}</dd></div>
          <div><dt>空闲</dt><dd class="mono dwz-silent-value" :class="{ 'is-refreshing': refreshing }">{{ status.db?.idle ?? 0 }}</dd></div>
          <div v-if="status.db?.error"><dt>错误</dt><dd>{{ status.db.error }}</dd></div>
        </dl>
      </section>

      <!-- Redis -->
      <section class="app-card">
        <h3 class="card-title">
          <el-icon><Connection /></el-icon>Redis
          <el-tag
            :type="status.redis?.healthy ? 'success' : 'danger'"
            size="small"
            round
            :aria-label="`Redis 状态：${status.redis?.healthy ? '正常' : '异常'}`"
          >
            {{ status.redis?.healthy ? '正常' : '异常' }}
          </el-tag>
        </h3>
        <dl class="kv">
          <div v-if="status.redis?.error"><dt>错误</dt><dd>{{ status.redis.error }}</dd></div>
          <div v-else><dt>状态</dt><dd>连接正常</dd></div>
        </dl>
      </section>

      <!-- 点击队列 -->
      <section class="app-card">
        <h3 class="card-title">
          <el-icon><Odometer /></el-icon>点击队列
        </h3>
        <dl class="kv">
          <div><dt>待处理</dt><dd class="mono dwz-silent-value" :class="{ 'is-refreshing': refreshing }">{{ status.queue?.pending ?? 0 }}</dd></div>
        </dl>
      </section>

      <!-- 定时任务：轮询时局部静默更新，不再整表遮罩闪烁 -->
      <section class="app-card wide">
        <h3 class="card-title"><el-icon><Timer /></el-icon>定时任务</h3>
        <div class="dwz-silent">
          <span v-if="refreshing" class="dwz-silent__bar" aria-hidden="true" />
          <el-table :data="status.cron ?? []" size="small" stripe>
            <el-table-column label="任务">
              <template #default="{ row }">
                {{ CRON_LABELS[row.name] ?? row.name }}
              </template>
            </el-table-column>
            <el-table-column label="上次运行" width="200">
              <template #default="{ row }">
                <span class="mono dwz-silent-value" :class="{ 'is-refreshing': refreshing }">{{ fmtTime(row.last_run) }}</span>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </section>
    </div>

    <el-empty v-else-if="initialized" description="暂无监控数据" />
  </div>
</template>

<style scoped>
.monitor-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 16px;
}

.monitor-grid .wide {
  grid-column: 1 / -1;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 14px;
  font-size: 15px;
  font-weight: 700;
  color: var(--dwz-ink);
}

.card-title .el-tag {
  margin-left: auto;
}

.kv {
  margin: 0;
}

.kv div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0;
  border-bottom: 1px solid var(--dwz-line);
}

.kv div:last-child {
  border-bottom: none;
}

.kv dt {
  font-size: 13px;
  color: var(--dwz-text-dim);
}

.kv dd {
  margin: 0;
  font-size: 13px;
  color: var(--dwz-ink);
}
</style>