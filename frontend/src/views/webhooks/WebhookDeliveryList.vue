<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { Search, Refresh, RefreshLeft } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  listWebhookDeliveries,
  retryWebhookDelivery,
  WEBHOOK_EVENTS,
  type WebhookDelivery
} from '@/api/webhooks'

const retrying = ref(false)

const loading = ref(false)
const rows = ref<WebhookDelivery[]>([])
const total = ref(0)

const query = reactive({
  page: 1,
  per_page: 20,
  webhook_id: undefined as number | undefined,
  event: '',
  result: '' as '' | 'success' | 'failed'
})

function eventLabel(v: string): string {
  return WEBHOOK_EVENTS.find((e) => e.value === v)?.label ?? v
}

async function loadData() {
  loading.value = true
  try {
    const res = await listWebhookDeliveries({
      page: query.page,
      per_page: query.per_page,
      webhook_id: query.webhook_id,
      event: query.event || undefined,
      result: query.result || undefined
    })
    rows.value = res?.list ?? []
    total.value = res?.total ?? 0
  } catch (err) {
    rows.value = []
    total.value = 0
    ElMessage.error(err instanceof Error ? err.message : '加载投递记录失败')
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  query.page = 1
  loadData()
}

function handleReset() {
  query.webhook_id = undefined
  query.event = ''
  query.result = ''
  handleSearch()
}

async function handleRetry(row: WebhookDelivery) {
  retrying.value = true
  try {
    const d = await retryWebhookDelivery(row.id)
    if (d.success === 1) {
      ElMessage.success(`重试成功：已收到 200 响应（新投递 #${d.id}）`)
    } else {
      ElMessage.warning(`重试失败：HTTP ${d.response_status || '无响应'}（新投递 #${d.id}）`)
    }
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '重试失败')
  } finally {
    retrying.value = false
  }
}

function prettyPayload(p: string): string {
  try {
    return JSON.stringify(JSON.parse(p), null, 2)
  } catch {
    return p
  }
}

onMounted(loadData)
</script>

<template>
  <section class="app-card">
    <div class="app-toolbar">
      <el-input
        v-model.number="query.webhook_id"
        placeholder="Webhook ID"
        clearable
        style="width: 140px"
        class="mono"
        @keyup.enter="handleSearch"
        @clear="handleSearch"
      />
      <el-select v-model="query.event" placeholder="事件" clearable style="width: 200px" @change="handleSearch">
        <el-option v-for="e in WEBHOOK_EVENTS" :key="e.value" :label="e.label" :value="e.value" />
      </el-select>
      <el-select v-model="query.result" placeholder="投递结果" clearable style="width: 140px" @change="handleSearch">
        <el-option label="成功" value="success" />
        <el-option label="失败" value="failed" />
      </el-select>
      <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
      <el-button :icon="Refresh" @click="handleReset">重置</el-button>
    </div>

    <div class="app-table-wrap">
      <el-table v-loading="loading" :data="rows" row-key="id" stripe>
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="detail">
              <div class="detail__head mono">PAYLOAD</div>
              <pre class="detail__json mono">{{ prettyPayload(row.payload) }}</pre>
              <div v-if="row.response_body" class="detail__head mono" style="margin-top: 10px">RESPONSE</div>
              <pre v-if="row.response_body" class="detail__resp mono">{{ row.response_body }}</pre>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="时间" width="170">
          <template #default="{ row }">
            <span class="mono cell-time">{{ dayjs(row.created_at).format('YYYY-MM-DD HH:mm:ss') }}</span>
          </template>
        </el-table-column>
        <el-table-column label="事件" width="170">
          <template #default="{ row }">
            <el-tag size="small" effect="plain" round>{{ eventLabel(row.event) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Webhook" width="110">
          <template #default="{ row }">
            <span class="mono cell-main">#{{ row.webhook_id }}</span>
          </template>
        </el-table-column>
        <el-table-column label="结果" width="110">
          <template #default="{ row }">
            <el-tag :type="row.success === 1 ? 'success' : 'danger'" size="small" effect="light" round>
              {{ row.success === 1 ? '成功' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="HTTP" width="90">
          <template #default="{ row }">
            <span class="mono cell-main">{{ row.response_status || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="尝试" width="80">
          <template #default="{ row }">
            <span class="mono cell-dim">第 {{ row.attempt }} 次</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right" align="center">
          <template #default="{ row }">
            <button
              v-if="row.success !== 1"
              class="mini-btn"
              :disabled="retrying"
              title="重新投递"
              @click="handleRetry(row as WebhookDelivery)"
            >
              <el-icon :size="13"><RefreshLeft /></el-icon>
            </button>
            <span v-else class="cell-dim">—</span>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无投递记录，访问短链或创建短链后会自动记录" />
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
  margin: 0;
  padding: 12px 14px;
  background: var(--dwz-ink);
  color: #b9e4dd;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.6;
  overflow-x: auto;
  max-width: 720px;
}

.detail__resp {
  margin: 0;
  padding: 12px 14px;
  background: #fff;
  border: 1px solid var(--dwz-line);
  color: var(--dwz-text);
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.6;
  overflow-x: auto;
  max-width: 720px;
}

.cell-time {
  font-size: 12.5px;
  color: var(--dwz-text);
}

.cell-main {
  font-size: 12.5px;
  color: var(--dwz-text);
}

.cell-dim {
  font-size: 12px;
  color: var(--dwz-text-dim);
}

.mini-btn {
  display: inline-grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border: 1px solid var(--dwz-line);
  border-radius: 7px;
  background: #fff;
  color: var(--dwz-text-dim);
  cursor: pointer;
  transition: all 0.15s ease;
}

.mini-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.mini-btn:hover:not(:disabled) {
  color: var(--dwz-petrol);
  border-color: var(--dwz-petrol);
  box-shadow: 0 3px 8px rgba(14, 110, 117, 0.14);
}
</style>