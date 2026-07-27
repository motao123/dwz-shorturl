<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { listAuditLogs, type AuditLog, type AuditLogQuery } from '@/api/audit'

const loading = ref(false)
const rows = ref<AuditLog[]>([])
const total = ref(0)

const query = reactive<AuditLogQuery>({
  page: 1,
  per_page: 20,
  user_id: '',
  action: '',
  date_from: '',
  date_to: ''
})

const dateRange = ref<[string, string] | null>(null)

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

async function loadData() {
  loading.value = true
  try {
    const params: AuditLogQuery = { ...query }
    if (dateRange.value) {
      params.date_from = dayjs(dateRange.value[0]).format('YYYY-MM-DD')
      params.date_to = dayjs(dateRange.value[1]).format('YYYY-MM-DD')
    } else {
      params.date_from = ''
      params.date_to = ''
    }
    const res = await listAuditLogs(params)
    if (Array.isArray(res)) {
      rows.value = res
      total.value = res.length
    } else {
      rows.value = res?.list ?? []
      total.value = res?.total ?? 0
    }
    // 补充动作选项
    for (const r of rows.value) {
      if (r.action && !actionOptions.value.includes(r.action)) {
        actionOptions.value.push(r.action)
      }
    }
  } catch (err) {
    rows.value = []
    total.value = 0
    ElMessage.error(err instanceof Error ? err.message : '加载审计日志失败')
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  query.page = 1
  loadData()
}

function handleReset() {
  query.user_id = ''
  query.action = ''
  dateRange.value = null
  handleSearch()
}

function formatDetail(detail: Record<string, unknown> | null): string {
  if (!detail) return '无'
  return JSON.stringify(detail, null, 2)
}

onMounted(loadData)
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
          v-model.number="query.user_id"
          placeholder="按用户 ID 筛选"
          clearable
          style="width: 160px"
          class="mono"
          @keyup.enter="handleSearch"
          @clear="handleSearch"
        />
        <el-select
          v-model="query.action"
          placeholder="操作类型"
          clearable
          filterable
          style="width: 180px"
          @change="handleSearch"
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
          @change="handleSearch"
        />
        <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
        <el-button @click="handleReset">重置</el-button>
      </div>

      <div class="app-table-wrap">
        <el-table v-loading="loading" :data="rows" row-key="id" stripe>
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
          v-model:current-page="query.page"
          v-model:page-size="query.per_page"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          background
          @current-change="loadData"
          @size-change="() => { query.page = 1; loadData() }"
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
