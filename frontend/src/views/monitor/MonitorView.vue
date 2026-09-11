<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { Refresh, Cpu, Connection, DataBoard, Timer, Odometer, Files } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import {
  getMonitorStatus,
  ensurePartitions,
  type MonitorStatus,
  type PartitionStatus
} from '@/api/monitor'

const loading = ref(false)
const status = ref<MonitorStatus | null>(null)
const partitionBusy = ref(false)

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

// --- click_logs 分区覆盖 ---

const partitions = computed<PartitionStatus | null>(() => status.value?.partitions ?? null)

/** 分区状态标签：ok 绿 / warning 黄 / critical 红 */
const partitionTagType = computed(() => {
  const level = partitions.value?.alert_level
  if (level === 'critical') return 'danger'
  if (level === 'warning') return 'warning'
  return 'success'
})

const partitionTagText = computed(() => {
  const level = partitions.value?.alert_level
  if (level === 'critical') return '异常'
  if (level === 'warning') return '需关注'
  return '正常'
})

/** 已覆盖区间：YYYY-MM ~ YYYY-MM */
const coverageText = computed(() => {
  const p = partitions.value
  if (!p) return '—'
  if (!p.available) return '未分区'
  if (!p.earliest_month || !p.latest_month) return '无月度分区'
  return `${fmtMonth(p.earliest_month)} ~ ${fmtMonth(p.latest_month)}`
})

/** 距目标月的差距描述 */
const behindText = computed(() => {
  const p = partitions.value
  if (!p) return '—'
  if (p.behind_months <= 0) return `已达标（目标 ${fmtMonth(p.required_month)}）`
  return `落后 ${p.behind_months} 个月（目标 ${fmtMonth(p.required_month)}）`
})

/** p_future 行数占比：兜底分区涨了说明月度分区没跟上 */
const futureText = computed(() => {
  const p = partitions.value
  if (!p) return '—'
  return `${p.future_rows} / ${p.total_rows}（${(p.future_ratio * 100).toFixed(2)}%）`
})

/**
 * 补齐分区：先让用户确认（大表有 DDL 锁风险），再执行幂等补齐。
 * 连续失败时会额外提示失败次数，避免运维在不了解状态时直接触发。
 */
async function handleEnsurePartitions() {
  const p = partitions.value
  const failures = p?.consecutive_failures ?? 0
  try {
    await ElMessageBox.confirm(
      failures > 0
        ? `分区维护任务已连续失败 ${failures} 次。ALTER TABLE 在大表上存在 DDL 锁风险，` +
          '建议先确认数据库负载与磁盘空间。将按目标月补齐缺失分区（幂等，已存在月份不会重复创建）。'
        : '将按目标月（当前月 + 2 个月）补齐缺失的 click_logs 月度分区，已存在的月份不会重复创建。',
      '补齐分区',
      { type: 'warning', confirmButtonText: '开始补齐', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  partitionBusy.value = true
  try {
    const res = await ensurePartitions()
    const created = res.created ?? []
    if (created.length > 0) {
      ElMessage.success(`已创建 ${created.length} 个分区：${created.map(fmtMonth).join('、')}`)
    } else {
      ElMessage.success('分区已覆盖至目标月，无需创建')
    }
    await load()
  } catch (err) {
    // 部分成功也会返回结构化结果：已创建的月份已落库，刷新后展示进度
    ElMessage.error(err instanceof Error ? err.message : '补齐分区未完成，请查看服务端日志')
    await load()
  } finally {
    partitionBusy.value = false
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
        <p class="app-page__desc">数据库 · Redis · 点击队列 · 定时任务 · 分区覆盖</p>
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
      <section class="app-card wide">
        <h3 class="card-title">
          <el-icon><Files /></el-icon>点击日志分区
          <el-tag
            :type="partitionTagType"
            size="small"
            round
            :aria-label="`点击日志分区状态：${partitionTagText}`"
          >
            {{ partitionTagText }}
          </el-tag>
          <el-button
            class="partition-fix"
            size="small"
            type="warning"
            plain
            :loading="partitionBusy"
            aria-label="补齐缺失的点击日志分区至目标月"
            @click="handleEnsurePartitions"
          >
            补齐分区
          </el-button>
        </h3>

        <!-- 覆盖不足时给出醒目提示：分区耗尽后新行会全部落入 p_future，
             click_logs 静默退化为单一大表，查询与清理的性能会持续劣化 -->
        <el-alert
          v-for="(msg, i) in partitions?.alerts ?? []"
          :key="i"
          :type="partitions?.alert_level === 'critical' ? 'error' : 'warning'"
          show-icon
          :closable="false"
          class="partition-alert"
          :title="msg"
        />

        <dl class="kv">
          <div>
            <dt>覆盖区间</dt>
            <dd class="mono">{{ coverageText }}</dd>
          </div>
          <div>
            <dt>覆盖目标</dt>
            <dd class="mono">{{ behindText }}</dd>
          </div>
          <div>
            <dt>p_future 行数占比</dt>
            <dd class="mono">{{ futureText }}</dd>
          </div>
          <div v-if="partitions?.pending_months?.length">
            <dt>待建分区</dt>
            <dd class="mono">{{ partitions?.pending_months?.map(fmtMonth).join('、') }}</dd>
          </div>
          <div>
            <dt>最近维护</dt>
            <dd class="mono">
              {{ fmtTime(partitions?.last_run_at) }}
              <template v-if="partitions?.created_last_run">
                （新建 {{ partitions?.created_last_run }} 个）
              </template>
            </dd>
          </div>
          <div v-if="partitions?.last_error">
            <dt>最近失败</dt>
            <dd>
              {{ partitions?.last_error }}
              <template v-if="partitions?.consecutive_failures">
                （连续 {{ partitions?.consecutive_failures }} 次）
              </template>
            </dd>
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
          <el-table-column label="状态" width="190">
            <template #default="{ row }">
              <el-tag v-if="row.failed" type="danger" size="small" round>
                失败{{ (row.consecutive_failures ?? 0) > 1 ? ` ×${row.consecutive_failures}` : '' }}
              </el-tag>
              <el-tag v-else type="success" size="small" round>正常</el-tag>
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