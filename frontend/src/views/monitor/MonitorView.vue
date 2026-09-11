<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { Refresh, Cpu, Connection, DataBoard, Timer, Odometer } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { getMonitorStatus, ensurePartitions, type MonitorStatus } from '@/api/monitor'

const loading = ref(false)
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

let timer: ReturnType<typeof setInterval> | null = null

async function load() {
  loading.value = true
  try {
    status.value = await getMonitorStatus()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '加载监控数据失败')
  } finally {
    loading.value = false
  }
}

/** 把 pYYYYMM 分区名渲染成 YYYY-MM 可读文本 */
function fmtMonth(name: string): string {
  return /^p\d{6}$/.test(name) ? `${name.slice(1, 5)}-${name.slice(5)}` : name
}

// 手动补齐分区：幂等，已覆盖的月份不会重复创建
async function handleEnsurePartitions() {
  try {
    await ElMessageBox.confirm(
      '将按后端默认目标（当前月 + 2 个月）补齐缺失的 click_logs 月度分区，已存在的月份不会重复创建。',
      '补齐分区',
      { type: 'warning', confirmButtonText: '开始补齐', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    const res = await ensurePartitions()
    ElMessage.success(res.created > 0 ? `已创建 ${res.created} 个分区` : '分区已覆盖，无需创建')
    await load()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '补齐分区失败')
  }
}

function fmtTime(iso: string | null | undefined): string {
  if (!iso) return '从未运行'
  return dayjs(iso).format('YYYY-MM-DD HH:mm:ss')
}

// 30s 自动刷新，组件卸载时清理
onMounted(() => {
  load()
  timer = setInterval(load, 30_000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
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
        @click="load"
      >
        刷新
      </el-button>
    </div>

    <div v-if="status" class="monitor-grid">
      <!-- 系统信息 -->
      <section class="app-card">
        <h3 class="card-title"><el-icon><Cpu /></el-icon>系统信息</h3>
        <dl class="kv">
          <div><dt>运行时长</dt><dd class="mono">{{ status.uptime }}</dd></div>
          <div><dt>启动时间</dt><dd class="mono">{{ fmtTime(status.start_time) }}</dd></div>
          <div><dt>Goroutine 数</dt><dd class="mono">{{ status.goroutines }}</dd></div>
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
          <div><dt>连接数</dt><dd class="mono">{{ status.db?.open_conns ?? 0 }}</dd></div>
          <div><dt>使用中</dt><dd class="mono">{{ status.db?.in_use ?? 0 }}</dd></div>
          <div><dt>空闲</dt><dd class="mono">{{ status.db?.idle ?? 0 }}</dd></div>
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
          <div><dt>待处理</dt><dd class="mono">{{ status.queue?.pending ?? 0 }}</dd></div>
        </dl>
      </section>

      <!-- click_logs 分区覆盖率 -->
      <section v-if="status.partitions" class="app-card wide">
        <h3 class="card-title">
          <el-icon><Timer /></el-icon>点击日志分区
          <el-tag
            :type="status.partitions.healthy ? 'success' : 'danger'"
            size="small"
            round
            :aria-label="`分区状态：${status.partitions.healthy ? '正常' : '异常'}`"
          >
            {{ status.partitions.healthy ? '正常' : '需关注' }}
          </el-tag>
          <el-button
            class="partition-fix"
            size="small"
            type="warning"
            plain
            aria-label="补齐缺失的点击日志分区"
            @click="handleEnsurePartitions"
          >
            补齐分区
          </el-button>
        </h3>
        <el-alert
          v-if="status.partitions.failing"
          type="error"
          show-icon
          :closable="false"
          class="partition-alert"
          title="分区维护连续失败"
          :description="`最近失败：${status.partitions.last_error || '未知错误'}（连续 ${status.partitions.consecutive_failures} 次）`"
        />
        <el-alert
          v-else-if="status.partitions.behind_months > 0"
          type="warning"
          show-icon
          :closable="false"
          class="partition-alert"
          title="分区覆盖落后于目标"
          :description="`已落后 ${status.partitions.behind_months} 个月，新数据可能落入 p_future 兜底分区，请点击「补齐分区」。`"
        />
        <dl class="kv">
          <div>
            <dt>覆盖月份</dt>
            <dd class="mono">
              {{ status.partitions.earliest_month ? fmtMonth(status.partitions.earliest_month) : '—' }}
              ~
              {{ status.partitions.latest_month ? fmtMonth(status.partitions.latest_month) : '—' }}
            </dd>
          </div>
          <div>
            <dt>目标月份</dt>
            <dd class="mono">{{ fmtMonth(status.partitions.required_month) }}</dd>
          </div>
          <div>
            <dt>落后月数</dt>
            <dd class="mono">{{ status.partitions.behind_months }}</dd>
          </div>
          <div>
            <dt>p_future 行数</dt>
            <dd class="mono">{{ status.partitions.future_rows }}</dd>
          </div>
          <div>
            <dt>最近维护</dt>
            <dd class="mono">{{ fmtTime(status.partitions.last_run_at) }}</dd>
          </div>
          <div>
            <dt>上次新建</dt>
            <dd class="mono">{{ status.partitions.created_last_run }} 个</dd>
          </div>
        </dl>
      </section>

      <!-- 定时任务 -->
      <section class="app-card wide">
        <h3 class="card-title"><el-icon><Timer /></el-icon>定时任务</h3>
        <el-table :data="status.cron ?? []" size="small" stripe>
          <el-table-column label="任务">
            <template #default="{ row }">
              {{ CRON_LABELS[row.name] ?? row.name }}
            </template>
          </el-table-column>
          <el-table-column label="上次运行" width="200">
            <template #default="{ row }">
              <span class="mono">{{ fmtTime(row.last_run) }}</span>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </div>

    <el-empty v-else-if="!loading" description="暂无监控数据" />
  </div>
</template>

<style scoped>
.card-title .partition-fix {
  margin-left: 12px;
}

.partition-alert {
  margin-bottom: 12px;
}

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