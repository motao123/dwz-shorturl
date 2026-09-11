<script setup lang="ts">
import { computed, ref } from 'vue'
import { Search } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { listAuditLogs, type AuditLog, type AuditLogQuery } from '@/api/audit'
import { useListPage } from '@/composables/useListPage'
import TableSkeleton from '@/components/TableSkeleton.vue'

/** 动作选项（可从数据中动态补充） */
const actionOptions = ref<string[]>([
  'auth.login',
  'auth.logout',
  'short_url.create',
  'short_url.update',
  'short_url.delete',
  'user.create',
  'user.update',
  'role.update',
  'config.update',
  'api_key.create',
  'api_key.revoke'
])

const ACTION_TEXT: Record<string, string> = {
  'auth.login': '登录',
  'auth.logout': '登出',
  'short_url.create': '创建短链',
  'short_url.update': '编辑短链',
  'short_url.delete': '删除短链',
  'user.create': '创建用户',
  'user.update': '编辑用户',
  'user.delete': '删除用户',
  'role.create': '创建角色',
  'role.update': '更新角色',
  'role.delete': '删除角色',
  'config.update': '修改配置',
  'api_key.create': '创建密钥',
  'api_key.revoke': '吊销密钥'
}

function actionTone(action: string): 'success' | 'warning' | 'danger' | 'info' | 'primary' {
  if (action.includes('delete') || action.includes('revoke')) return 'danger'
  if (action.includes('create')) return 'success'
  if (action.includes('update')) return 'warning'
  if (action.startsWith('auth')) return 'info'
  return 'primary'
}

/**
 * 日期范围在状态上拆成 date_start / date_end 两个筛选字段（便于统一进 URL），
 * 组件里用 computed 双向代理给 el-date-picker 的 [start, end] 数组，
 * 请求参数转换则集中在 buildRequestParams 一处，迁移前后请求形态不变。
 */
const page = useListPage<AuditLog>({
  perPage: 20,
  perPageOptions: [20, 50, 100],
  filters: {
    user_id: '' as number | '',
    action: '',
    date_start: '',
    date_end: ''
  },
  fetcher: (params) => listAuditLogs(buildRequestParams(params)),
  errorMessage: '加载审计日志失败',
  onLoaded: (list) => {
    // 补充动作选项：历史数据里可能有未预置的 action
    for (const r of list) {
      if (r.action && !actionOptions.value.includes(r.action)) {
        actionOptions.value.push(r.action)
      }
    }
  }
})

/**
 * 请求参数组装：页面独有的派生逻辑集中在这里，
 * useListQuery 只管分页与「原样筛选字段」的透传。
 * URL 里存的是 date_start/date_end，请求侧仍按既有契约发 date_from/date_to。
 */
function buildRequestParams(params: Record<string, unknown>): AuditLogQuery {
  const next = { ...params } as AuditLogQuery & { date_start?: string; date_end?: string }
  delete next.date_start
  delete next.date_end
  next.date_from = dateStart.value ? dayjs(String(dateStart.value)).format('YYYY-MM-DD') : ''
  next.date_to = dateEnd.value ? dayjs(String(dateEnd.value)).format('YYYY-MM-DD') : ''
  return next
}

/** 日期范围：date_start/date_end ↔ el-date-picker 的 [start, end] */
const dateRange = computed({
  get: (): [string, string] | null =>
    dateStart.value && dateEnd.value
      ? ([String(dateStart.value), String(dateEnd.value)] as [string, string])
      : null,
  set: (value: [string, string] | null) => {
    page.setFilters({ date_start: value?.[0] ?? '', date_end: value?.[1] ?? '' })
  }
})

/** 日期变化：回到第 1 页重新查询（computed setter 已把范围写入筛选状态） */
function handleDateChange() {
  page.setFilters({})
  void page.load()
}

/** 重置：清空日期范围与全部筛选，回到第 1 页重新查询 */
function handleReset() {
  page.setFilters({})
  page.resetFilters()
  void page.search()
}

function formatDetail(detail: Record<string, unknown> | null): string {
  if (!detail) return '无'
  return JSON.stringify(detail, null, 2)
}

const {
  page: currentPage,
  perPage,
  rows,
  total,
  loading,
  hasLoaded,
  refreshing,
  pagerLayout,
  search,
  handlePageChange,
  handleSizeChange
} = page

/** 首屏骨架：仅首次加载时展示，后续翻页/筛选保留旧内容（仅顶部细进度条） */
const showSkeleton = computed(() => loading.value && !hasLoaded.value)
/** 骨架行数取 page-size，尺寸与真实表格一致，避免 CLS */
const skeletonRows = computed(() => Math.min(perPage.value || 20, 20))
/** 骨架列宽与真实列（展开 / 时间 / 操作人 / 操作 / 资源 / IP）对齐 */
const skeletonWidths = ['36px', '150px', '110px', '130px', 'minmax(160px, 1fr)', '120px']

// 模板里用的 v-model 代理：变更后自动回到第 1 页并同步 URL，回车/清空再触发查询
const userIdFilter = page.filterRef<number | ''>('user_id')
const actionFilter = page.filterRef<string>('action')
const dateStart = page.filterRef<string>('date_start')
const dateEnd = page.filterRef<string>('date_end')

/**
 * 从地址栏还原筛选与日期范围。
 * 必须在 useListPage 的 onMounted 加载之前执行，否则首次请求会丢掉 URL 里的条件。
 */
page.hydrateFromUrl()
</script>


<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          审计日志
          <small>AUDIT LOGS · 仅追加 · 保留 90 天</small>
        </h1>
        <p class="app-page__desc">共 {{ total.toLocaleString() }} 条操作记录</p>
      </div>
    </div>

    <section class="app-card">
      <div class="app-toolbar">
        <el-input
          v-model="userIdFilter"
          placeholder="按用户 ID 筛选"
          clearable
          style="width: 160px"
          class="mono"
          @keyup.enter="search"
          @clear="search"
        />
        <el-select
          v-model="actionFilter"
          placeholder="操作类型"
          clearable
          filterable
          style="width: 180px"
          @change="search"
        >
          <el-option
            v-for="a in actionOptions"
            :key="a"
            :label="ACTION_TEXT[a] ? `${ACTION_TEXT[a]}（${a}）` : a"
            :value="a"
          />
        </el-select>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          value-format="YYYY-MM-DD"
          style="width: 250px"
          @change="handleDateChange"
        />
        <el-button type="primary" :icon="Search" aria-label="按筛选条件查询审计日志" @click="search">
          查询
        </el-button>
        <el-button aria-label="重置筛选条件" @click="handleReset">重置</el-button>
      </div>

      <div class="app-table-wrap dwz-silent" :class="{ 'is-loading': showSkeleton }">
        <span v-if="refreshing" class="dwz-silent__bar" aria-hidden="true" />
        <TableSkeleton v-if="showSkeleton" :rows="skeletonRows" :widths="skeletonWidths" />
        <el-table v-show="!showSkeleton" :data="rows" row-key="id" stripe>
          <el-table-column type="expand">
            <template #default="{ row }">
              <div class="detail">
                <div class="detail__head mono">操作详情快照 DETAIL</div>
                <pre class="detail__json mono">{{ formatDetail(row.detail) }}</pre>
                <div v-if="row.user_agent" class="detail__ua mono">UA · {{ row.user_agent }}</div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="时间" width="170">
            <template #default="{ row }">
              <span class="mono cell-time">{{ dayjs(row.created_at).format('YYYY-MM-DD HH:mm:ss') }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作人" width="130">
            <template #default="{ row }">
              <span v-if="row.username" class="cell-user">{{ row.username }}</span>
              <span v-else-if="row.user_id" class="mono cell-user">#{{ row.user_id }}</span>
              <span v-else class="cell-system">系统</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150">
            <template #default="{ row }">
              <el-tag :type="actionTone(row.action)" size="small" effect="light" round>
                {{ ACTION_TEXT[row.action] ?? row.action }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="资源" min-width="180">
            <template #default="{ row }">
              <span class="mono cell-res">
                {{ row.resource ?? '—' }}<template v-if="row.resource_id"> / {{ row.resource_id }}</template>
              </span>
            </template>
          </el-table-column>
          <el-table-column label="来源 IP" width="140">
            <template #default="{ row }">
              <span class="mono cell-ip">{{ row.ip }}</span>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无审计日志" />
          </template>
        </el-table>
      </div>

      <div class="app-pager">
        <el-pagination
          :current-page="currentPage"
          :page-size="perPage"
          :total="total"
          :page-sizes="page.perPageOptions"
          :layout="pagerLayout"
          background
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </section>
  </div>
</template>

<style scoped>
.detail {
  padding: 12px 20px 16px 64px;
  background: #f7fafb;
}

.detail__head {
  font-size: 10px;
  letter-spacing: 0.2em;
  color: var(--dwz-text-dim);
  margin-bottom: 8px;
}

.detail__json {
  margin: 0 0 10px;
  padding: 12px 14px;
  background: var(--dwz-ink);
  color: #b9e4dd;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.6;
  overflow-x: auto;
  max-width: 720px;
}

.detail__ua {
  font-size: 11px;
  color: var(--dwz-text-dim);
  word-break: break-all;
}

.cell-time {
  font-size: 12.5px;
  color: var(--dwz-text);
}

.cell-user {
  font-weight: 700;
  color: var(--dwz-ink);
}

.cell-system {
  color: var(--dwz-text-dim);
  font-style: italic;
}

.cell-res {
  font-size: 12.5px;
  color: var(--dwz-text);
}

.cell-ip {
  font-size: 12px;
  color: var(--dwz-text-dim);
}
</style>
