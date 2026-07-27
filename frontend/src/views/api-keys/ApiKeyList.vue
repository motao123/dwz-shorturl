<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, DocumentCopy, CircleClose, DataLine, WarningFilled } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  listApiKeys,
  createApiKey,
  revokeApiKey,
  getApiKeyStats,
  type ApiKey,
  type ApiKeyStats
} from '@/api/api-keys'
import { API_KEY_STATUS } from '@/utils/constants'
import { copyText } from '@/utils/clipboard'

const loading = ref(false)
const rows = ref<ApiKey[]>([])
const total = ref(0)
const query = reactive({ page: 1, per_page: 20 })

/* ---------------- 创建 ---------------- */

const createVisible = ref(false)
const creating = ref(false)
const formRef = ref()
const form = reactive({
  name: '',
  rate_limit: 100,
  expires_days: null as number | null
})

const rules = {
  name: [
    { required: true, message: '请输入密钥用途名称', trigger: 'blur' },
    { max: 64, message: '名称不能超过 64 个字符', trigger: 'blur' }
  ],
  rate_limit: [{ required: true, message: '请设置每分钟限额', trigger: 'blur' }]
}

/* 创建成功结果（明文仅展示一次） */
const resultVisible = ref(false)
const createdKey = ref('')
const createdName = ref('')

/* 统计弹窗 */
const statsVisible = ref(false)
const statsLoading = ref(false)
const statsData = ref<ApiKeyStats | null>(null)
const statsKeyName = ref('')

async function loadData() {
  loading.value = true
  try {
    const res = await listApiKeys({ ...query })
    if (Array.isArray(res)) {
      rows.value = res
      total.value = res.length
    } else {
      rows.value = res?.list ?? []
      total.value = res?.total ?? 0
    }
  } catch (err) {
    rows.value = []
    total.value = 0
    ElMessage.error(err instanceof Error ? err.message : '加载密钥列表失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, { name: '', rate_limit: 100, expires_days: null })
  createVisible.value = true
}

async function handleCreate() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  creating.value = true
  try {
    const res = await createApiKey({
      name: form.name.trim(),
      rate_limit: Number(form.rate_limit),
      expires_days: form.expires_days
    })
    createdKey.value = res.api_key
    createdName.value = res.name
    createVisible.value = false
    resultVisible.value = true
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '创建密钥失败')
  } finally {
    creating.value = false
  }
}

async function copyCreatedKey() {
  try {
    await copyText(createdKey.value)
    ElMessage.success('密钥已复制，请妥善保管')
  } catch {
    ElMessage.error('复制失败，请手动选择复制')
  }
}

async function handleRevoke(row: ApiKey) {
  try {
    await ElMessageBox.confirm(
      `确定吊销密钥「${row.name}」吗？吊销后使用该密钥的所有 API 调用将立即失败，且不可恢复。`,
      '吊销确认',
      { confirmButtonText: '吊销', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await revokeApiKey(row.id)
    ElMessage.success('密钥已吊销')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '吊销失败')
  }
}

async function openStats(row: ApiKey) {
  statsKeyName.value = row.name
  statsData.value = null
  statsVisible.value = true
  statsLoading.value = true
  try {
    statsData.value = await getApiKeyStats(row.id)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '加载密钥统计失败')
  } finally {
    statsLoading.value = false
  }
}

const statsMax = () =>
  Math.max(1, ...(statsData.value?.recent?.map((r) => r.count) ?? [1]))

onMounted(loadData)
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          API 密钥
          <small>API KEYS · 开放接口凭证</small>
        </h1>
        <p class="app-page__desc">共 {{ total }} 个密钥 · 明文仅在创建时展示一次</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">创建密钥</el-button>
    </div>

    <section class="app-card">
      <div class="app-table-wrap" style="padding-top: 16px">
        <el-table v-loading="loading" :data="rows" row-key="id" stripe>
          <el-table-column label="名称" min-width="180">
            <template #default="{ row }">
              <span class="key-name">{{ row.name }}</span>
            </template>
          </el-table-column>
          <el-table-column label="密钥前缀" width="170">
            <template #default="{ row }">
              <span class="mono key-prefix">{{ row.key_prefix }}••••••••</span>
            </template>
          </el-table-column>
          <el-table-column label="限流（次/分）" width="130" align="right">
            <template #default="{ row }">
              <span class="mono key-rate">{{ row.rate_limit }}</span>
            </template>
          </el-table-column>
          <el-table-column label="最近使用" width="165">
            <template #default="{ row }">
              <span v-if="row.last_used_at" class="mono cell-dim">
                {{ dayjs(row.last_used_at).format('YYYY-MM-DD HH:mm') }}
              </span>
              <span v-else class="cell-dim">从未使用</span>
            </template>
          </el-table-column>
          <el-table-column label="过期时间" width="125">
            <template #default="{ row }">
              <span class="mono" :class="row.expires_at ? '' : 'forever'">
                {{ row.expires_at ? dayjs(row.expires_at).format('YYYY-MM-DD') : '永久' }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="92" align="center">
            <template #default="{ row }">
              <el-tag :type="API_KEY_STATUS[row.status as 0 | 1]?.type ?? 'info'" size="small" round>
                {{ API_KEY_STATUS[row.status as 0 | 1]?.label ?? '未知' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="130" fixed="right" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-tooltip content="调用统计" placement="top">
                  <button class="mini-btn" @click="openStats(row as ApiKey)">
                    <el-icon :size="13"><DataLine /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="吊销" placement="top">
                  <span>
                    <button
                      class="mini-btn mini-btn--danger"
                      :disabled="row.status === 0"
                      @click="handleRevoke(row as ApiKey)"
                    >
                      <el-icon :size="13"><CircleClose /></el-icon>
                    </button>
                  </span>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无 API 密钥，点击右上角创建" />
          </template>
        </el-table>
      </div>

      <div class="app-pager">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.per_page"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          background
          @current-change="loadData"
          @size-change="() => { query.page = 1; loadData() }"
        />
      </div>
    </section>

    <!-- 创建弹窗 -->
    <el-dialog v-model="createVisible" title="创建 API 密钥" width="480px" :close-on-click-modal="false" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="密钥用途" prop="name">
          <el-input v-model="form.name" placeholder="如：小程序后端调用" maxlength="64" show-word-limit />
        </el-form-item>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="限流（次/分钟）" prop="rate_limit">
              <el-input-number v-model="form.rate_limit" :min="1" :max="100000" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="有效期">
              <el-select v-model="form.expires_days" placeholder="永久有效" clearable style="width: 100%">
                <el-option label="30 天" :value="30" />
                <el-option label="90 天" :value="90" />
                <el-option label="180 天" :value="180" />
                <el-option label="1 年" :value="365" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="handleCreate">创建密钥</el-button>
      </template>
    </el-dialog>

    <!-- 明文结果弹窗（仅一次） -->
    <el-dialog
      v-model="resultVisible"
      title="密钥创建成功"
      width="560px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="true"
    >
      <div class="key-result">
        <div class="key-result__warn">
          <el-icon :size="17"><WarningFilled /></el-icon>
          完整密钥仅展示这一次，关闭后无法再次查看，请立即复制保存。
        </div>
        <div class="key-result__name">{{ createdName }}</div>
        <div class="key-result__box">
          <code class="mono">{{ createdKey }}</code>
          <el-button type="primary" :icon="DocumentCopy" @click="copyCreatedKey">复制</el-button>
        </div>
      </div>
      <template #footer>
        <el-button type="primary" @click="copyCreatedKey">复制并关闭前请确认已保存</el-button>
        <el-button @click="resultVisible = false">我已保存，关闭</el-button>
      </template>
    </el-dialog>

    <!-- 统计弹窗 -->
    <el-dialog v-model="statsVisible" :title="`调用统计 · ${statsKeyName}`" width="460px">
      <div v-loading="statsLoading" class="key-stats">
        <template v-if="statsData">
          <div class="key-stats__row">
            <div class="key-stats__num">
              <span class="mono">{{ statsData.total_requests.toLocaleString() }}</span>
              <small>累计调用</small>
            </div>
            <div class="key-stats__num">
              <span class="mono">{{ statsData.today_requests.toLocaleString() }}</span>
              <small>今日调用</small>
            </div>
          </div>
          <div v-if="statsData.recent?.length" class="key-stats__bars">
            <div v-for="r in statsData.recent" :key="r.label" class="bar-row">
              <span class="mono bar-label">{{ r.label }}</span>
              <div class="bar-track">
                <div class="bar-fill" :style="{ width: `${(r.count / statsMax()) * 100}%` }"></div>
              </div>
              <span class="mono bar-count">{{ r.count.toLocaleString() }}</span>
            </div>
          </div>
          <el-empty v-else description="暂无调用记录" :image-size="60" />
        </template>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.key-name {
  font-weight: 700;
  color: var(--dwz-ink);
}

.key-prefix {
  font-size: 12.5px;
  color: var(--dwz-petrol-strong);
  background: rgba(14, 110, 117, 0.07);
  padding: 2px 8px;
  border-radius: 6px;
}

.key-rate {
  font-weight: 700;
  color: var(--dwz-ink);
}

.cell-dim {
  font-size: 12px;
  color: var(--dwz-text-dim);
}

.forever {
  color: var(--dwz-good);
  font-weight: 600;
}

/* 明文结果 */
.key-result__warn {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 11px 14px;
  border-radius: 9px;
  background: rgba(220, 38, 38, 0.07);
  border: 1px solid rgba(220, 38, 38, 0.25);
  color: #b91c1c;
  font-size: 13px;
  line-height: 1.55;
  margin-bottom: 16px;
}

.key-result__name {
  font-weight: 800;
  color: var(--dwz-ink);
  margin-bottom: 8px;
}

.key-result__box {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 13px 15px;
  border-radius: 10px;
  background: var(--dwz-ink);
}

.key-result__box code {
  flex: 1;
  color: #ffd58a;
  font-size: 13px;
  word-break: break-all;
  line-height: 1.6;
  user-select: all;
}

/* 统计弹窗 */
.key-stats__row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-bottom: 18px;
}

.key-stats__num {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  padding: 14px 0;
  border-radius: 10px;
  background: #f4f8f8;
  border: 1px solid var(--dwz-line);
}

.key-stats__num span {
  font-size: 24px;
  font-weight: 700;
  color: var(--dwz-ink);
}

.key-stats__num small {
  font-size: 11.5px;
  color: var(--dwz-text-dim);
}

.bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.bar-label {
  width: 76px;
  font-size: 11px;
  color: var(--dwz-text-dim);
  flex-shrink: 0;
}

.bar-track {
  flex: 1;
  height: 10px;
  border-radius: 5px;
  background: #eef3f4;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 5px;
  background: linear-gradient(90deg, #0e6e75, #2fa3a8);
  transition: width 0.5s cubic-bezier(0.22, 1, 0.36, 1);
  min-width: 2px;
}

.bar-count {
  width: 62px;
  text-align: right;
  font-size: 11.5px;
  font-weight: 700;
  color: var(--dwz-ink);
  flex-shrink: 0;
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

.mini-btn:hover:not(:disabled) {
  color: var(--dwz-petrol);
  border-color: var(--dwz-petrol);
  transform: translateY(-1px);
  box-shadow: 0 3px 8px rgba(14, 110, 117, 0.15);
}

.mini-btn--danger:hover:not(:disabled) {
  color: var(--dwz-bad);
  border-color: var(--dwz-bad);
  box-shadow: 0 3px 8px rgba(220, 38, 38, 0.14);
}

.mini-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.ops {
  display: inline-flex;
  gap: 6px;
  justify-content: center;
}

:deep(.el-form-item__label) {
  font-weight: 700;
  color: var(--dwz-ink);
}
</style>
