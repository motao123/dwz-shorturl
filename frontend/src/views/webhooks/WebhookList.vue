<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { Plus, Delete, Refresh, Promotion } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import WebhookDeliveryList from './WebhookDeliveryList.vue'
import {
  listWebhooks,
  createWebhook,
  removeWebhook,
  pingWebhook,
  WEBHOOK_EVENTS,
  type WebhookSub
} from '@/api/webhooks'

const pinging = ref(false)

const activeTab = ref('subs')

const loading = ref(false)
const rows = ref<WebhookSub[]>([])

const dialogVisible = ref(false)
const creating = ref(false)
const formRef = ref()
const form = reactive({
  name: '',
  url: '',
  events: ['link.created'] as string[],
  secret: ''
})

const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  url: [
    { required: true, message: '请输入回调 URL', trigger: 'blur' },
    { type: 'url' as const, message: 'URL 格式不正确', trigger: 'blur' }
  ],
  events: [{ required: true, message: '请选择事件', trigger: 'change' }]
}

async function loadData() {
  loading.value = true
  try {
    const res = await listWebhooks()
    rows.value = Array.isArray(res) ? res : []
  } catch (err) {
    rows.value = []
    ElMessage.error(err instanceof Error ? err.message : '加载 Webhook 失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, { name: '', url: '', events: ['link.created'], secret: '' })
  dialogVisible.value = true
}

async function handleSubmit() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  creating.value = true
  try {
    await createWebhook({
      name: form.name.trim(),
      url: form.url.trim(),
      events: form.events,
      secret: form.secret.trim() || undefined
    })
    ElMessage.success('Webhook 已创建')
    dialogVisible.value = false
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '创建失败')
  } finally {
    creating.value = false
  }
}

async function handlePing(row: WebhookSub) {
  pinging.value = true
  try {
    const d = await pingWebhook(row.id)
    if (d.success === 1) {
      ElMessage.success('测试成功：已收到 200 响应')
    } else {
      ElMessage.warning(`测试失败：HTTP ${d.response_status || '无响应'}（已记录投递 #${d.id}）`)
    }
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '测试失败')
  } finally {
    pinging.value = false
  }
}

async function handleRemove(row: WebhookSub) {
  try {
    await ElMessageBox.confirm(
      `确定删除 Webhook「${row.name}」吗？删除后不再接收事件通知。`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await removeWebhook(row.id)
    ElMessage.success('已删除')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '删除失败')
  }
}

function eventLabel(v: string): string {
  return WEBHOOK_EVENTS.find((e) => e.value === v)?.label ?? v
}

onMounted(loadData)
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          Webhook
          <small>WEBHOOK · 事件通知</small>
        </h1>
        <p class="app-page__desc">短链创建 / 点击时向回调地址推送事件，支持签名与投递重试</p>
      </div>
      <div class="head-actions">
        <el-button :icon="Refresh" @click="loadData">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建 Webhook</el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="app-tabs">
      <el-tab-pane label="订阅列表" name="subs">
        <section class="app-card">
          <div class="app-table-wrap">
            <el-table v-loading="loading" :data="rows" row-key="id" stripe>
          <el-table-column prop="name" label="名称" min-width="140" />
          <el-table-column prop="url" label="回调 URL" min-width="260">
            <template #default="{ row }">
              <span class="mono">{{ row.url }}</span>
            </template>
          </el-table-column>
          <el-table-column label="事件" min-width="180">
            <template #default="{ row }">
              <el-tag v-for="e in row.events" :key="e" size="small" effect="plain" round class="evt">
                {{ eventLabel(e) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">
              <span class="mono">{{ dayjs(row.created_at).format('YYYY-MM-DD HH:mm') }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="140" fixed="right" align="center">
            <template #default="{ row }">
              <div class="op-row">
                <button class="mini-btn" :disabled="pinging" title="测试 Ping" @click="handlePing(row as WebhookSub)">
                  <el-icon :size="13"><Promotion /></el-icon>
                </button>
                <button class="mini-btn mini-btn--danger" @click="handleRemove(row as WebhookSub)">
                  <el-icon :size="13"><Delete /></el-icon>
                </button>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无 Webhook，点击右上角创建" />
          </template>
        </el-table>
          </div>
        </section>
      </el-tab-pane>
      <el-tab-pane label="投递记录" name="deliveries">
        <WebhookDeliveryList />
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="dialogVisible" title="新建 Webhook" width="520px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="如：通知服务" />
        </el-form-item>
        <el-form-item label="回调 URL" prop="url">
          <el-input v-model="form.url" placeholder="https://example.com/webhook" class="mono" />
        </el-form-item>
        <el-form-item label="事件" prop="events">
          <el-checkbox-group v-model="form.events">
            <el-checkbox v-for="e in WEBHOOK_EVENTS" :key="e.value" :value="e.value">
              {{ e.label }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item label="签名密钥（可选）">
          <el-input v-model="form.secret" placeholder="用于 HMAC-SHA256 签名验证" class="mono" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="handleSubmit">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.app-tabs {
  margin-top: 4px;
}

.app-tabs :deep(.el-tabs__header) {
  margin-bottom: 16px;
}

.evt {
  margin-right: 6px;
}

.op-row {
  display: inline-flex;
  gap: 6px;
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

.mini-btn--danger:hover {
  color: var(--dwz-bad);
  border-color: var(--dwz-bad);
  box-shadow: 0 3px 8px rgba(220, 38, 38, 0.14);
}
</style>