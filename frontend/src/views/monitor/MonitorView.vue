<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { Refresh, Cpu, Connection, DataBoard, Timer, Odometer } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { getMonitorStatus, type MonitorStatus } from '@/api/monitor'

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

function fmtTime(iso: string | null): string {
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
      <el-button type="primary" :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
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
          <el-tag :type="status.db?.healthy ? 'success' : 'danger'" size="small" round>
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
          <el-tag :type="status.redis?.healthy ? 'success' : 'danger'" size="small" round>
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