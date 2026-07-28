<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Search, Plus, EditPen, Delete, Refresh } from '@element-plus/icons-vue'
import {
  listDomains,
  createDomain,
  updateDomain,
  deleteDomain,
  checkDomain,
  batchUpdateStatus,
  type Domain,
  type DomainPayload
} from '@/api/domains'

const loading = ref(false)
const rows = ref<Domain[]>([])
const selected = ref<Domain[]>([])

const searchKeyword = ref('')
const statusFilter = ref<number | ''>('')

/* ---------------- 弹窗 ---------------- */

const dialogVisible = ref(false)
const submitting = ref(false)
const editingRow = ref<Domain | null>(null)
const isEdit = computed(() => Boolean(editingRow.value))

const formRef = ref<FormInstance>()

interface FormState {
  domain: string
  scheme: string
  name: string
  project: string
  priority: number
  status: 0 | 1
}

const form = reactive<FormState>({
  domain: '',
  scheme: 'https',
  name: '',
  project: '',
  priority: 50,
  status: 1
})

const rules: FormRules = {
  domain: [{ required: true, message: '请输入域名', trigger: 'blur' }],
  scheme: [{ required: true, message: '请选择协议', trigger: 'change' }]
}

/* ---------------- 状态映射 ---------------- */

const STATUS_MAP: Record<number, { label: string; type: 'success' | 'info' | 'danger' }> = {
  1: { label: '启用', type: 'success' },
  0: { label: '停用', type: 'info' },
  2: { label: '异常', type: 'danger' }
}

const CHECK_MAP: Record<string, { label: string; type: 'success' | 'info' | 'danger' }> = {
  ok: { label: '正常', type: 'success' },
  fail: { label: '失败', type: 'danger' },
  pending: { label: '待检测', type: 'info' }
}

/* ---------------- 数据加载 ---------------- */

async function loadData() {
  loading.value = true
  try {
    const res = await listDomains(
      statusFilter.value !== '' ? (statusFilter.value as number) : undefined
    )
    rows.value = Array.isArray(res) ? res : []
  } catch (err) {
    rows.value = []
    ElMessage.error(err instanceof Error ? err.message : '加载域名列表失败')
  } finally {
    loading.value = false
  }
}

const filteredRows = computed<Domain[]>(() => {
  const kw = searchKeyword.value.trim().toLowerCase()
  if (!kw) return rows.value
  return rows.value.filter(
    (r) =>
      r.domain.toLowerCase().includes(kw) ||
      (r.name ?? '').toLowerCase().includes(kw)
  )
})

/* ---------------- 增删改 ---------------- */

function resetForm() {
  form.domain = ''
  form.scheme = 'https'
  form.name = ''
  form.project = ''
  form.priority = 50
  form.status = 1
  formRef.value?.clearValidate()
}

function openCreate() {
  editingRow.value = null
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: Domain) {
  editingRow.value = row
  form.domain = row.domain
  form.scheme = row.scheme
  form.name = row.name ?? ''
  form.project = row.project ?? ''
  form.priority = row.priority ?? 50
  form.status = row.status === 1 ? 1 : 0
  dialogVisible.value = true
}

async function handleSubmit() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  submitting.value = true
  try {
    const payload: DomainPayload = {
      domain: form.domain.trim(),
      scheme: form.scheme,
      name: form.name.trim() || undefined,
      project: form.project.trim() || undefined,
      priority: form.priority,
      status: form.status
    }
    if (isEdit.value && editingRow.value) {
      await updateDomain(editingRow.value.id, payload)
      ElMessage.success('域名已更新')
    } else {
      await createDomain(payload)
      ElMessage.success('域名创建成功')
    }
    dialogVisible.value = false
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    submitting.value = false
  }
}

async function handleRemove(row: Domain) {
  try {
    await ElMessageBox.confirm(
      `确定删除域名「${row.domain}」吗？该操作不可撤销。`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await deleteDomain(row.id)
    ElMessage.success('域名已删除')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '删除失败')
  }
}

/* ---------------- 检测 ---------------- */

async function handleCheck(row: Domain) {
  try {
    await checkDomain(row.id)
    ElMessage.success(`已触发域名「${row.domain}」的检测，结果已刷新`)
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '检测失败')
  }
}

/* ---------------- 批量状态 ---------------- */

async function handleBatchStatus(status: number) {
  if (!selected.value.length) return
  const label = status === 1 ? '启用' : '停用'
  try {
    await ElMessageBox.confirm(
      `确定批量${label}选中的 ${selected.value.length} 个域名吗？`,
      `批量${label}`,
      { confirmButtonText: `批量${label}`, cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await batchUpdateStatus(
      selected.value.map((r) => r.id),
      status
    )
    ElMessage.success(`已批量${label} ${selected.value.length} 个域名`)
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : `批量${label}失败`)
  }
}

onMounted(loadData)
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          域名管理
          <small>DOMAINS · 短链域名池配置</small>
        </h1>
        <p class="app-page__desc">共 {{ rows.length }} 个域名</p>
      </div>
      <div class="head-actions">
        <el-button
          :icon="Refresh"
          :disabled="!selected.length"
          @click="handleBatchStatus(1)"
        >
          批量启用<span v-if="selected.length" class="mono">&nbsp;({{ selected.length }})</span>
        </el-button>
        <el-button
          :icon="Refresh"
          :disabled="!selected.length"
          @click="handleBatchStatus(0)"
        >
          批量停用<span v-if="selected.length" class="mono">&nbsp;({{ selected.length }})</span>
        </el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">添加域名</el-button>
      </div>
    </div>

    <section class="app-card">
      <!-- 筛选条 -->
      <div class="app-toolbar">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索域名 / 备注"
          :prefix-icon="Search"
          clearable
          style="width: 260px"
        />
        <el-select
          v-model="statusFilter"
          placeholder="状态"
          clearable
          style="width: 130px"
          @change="loadData"
        >
          <el-option label="启用" :value="1" />
          <el-option label="停用" :value="0" />
          <el-option label="异常" :value="2" />
        </el-select>
      </div>

      <!-- 表格 -->
      <div class="app-table-wrap">
        <el-table
          v-loading="loading"
          :data="filteredRows"
          row-key="id"
          stripe
          @selection-change="(val: Domain[]) => (selected = val)"
        >
          <el-table-column type="selection" width="44" />
          <el-table-column label="域名" min-width="200">
            <template #default="{ row }">
              <span class="domain mono">{{ row.domain }}</span>
              <div v-if="row.name" class="row-sub">{{ row.name }}</div>
            </template>
          </el-table-column>
          <el-table-column label="协议" width="80" align="center">
            <template #default="{ row }">
              <el-tag size="small" effect="plain" round>{{ row.scheme }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="项目" min-width="100">
            <template #default="{ row }">
              <span class="row-sub">{{ row.project || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="80" align="center">
            <template #default="{ row }">
              <el-tag
                :type="STATUS_MAP[row.status as 0 | 1 | 2]?.type ?? 'info'"
                size="small"
                effect="light"
                round
              >
                {{ STATUS_MAP[row.status as 0 | 1 | 2]?.label ?? '未知' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="优先级" width="70" align="center">
            <template #default="{ row }">
              <span class="mono priority">{{ row.priority }}</span>
            </template>
          </el-table-column>
          <el-table-column label="健康" width="120" align="center">
            <template #default="{ row }">
              <div class="health-cell">
                <el-tag
                  :type="CHECK_MAP[row.dns_status]?.type ?? 'info'"
                  size="small"
                  effect="plain"
                  round
                >
                  DNS {{ CHECK_MAP[row.dns_status]?.label ?? '-' }}
                </el-tag>
                <el-tag
                  :type="CHECK_MAP[row.ssl_status]?.type ?? 'info'"
                  size="small"
                  effect="plain"
                  round
                >
                  SSL {{ CHECK_MAP[row.ssl_status]?.label ?? '-' }}
                </el-tag>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="链接数" width="80" align="right">
            <template #default="{ row }">
              <span class="mono clicks">{{ row.link_count ?? 0 }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="122" fixed="right" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-tooltip content="编辑" placement="top">
                  <button class="mini-btn" @click="openEdit(row as Domain)">
                    <el-icon :size="13"><EditPen /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="检测" placement="top">
                  <button class="mini-btn" @click="handleCheck(row as Domain)">
                    <el-icon :size="13"><Refresh /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="删除" placement="top">
                  <button class="mini-btn mini-btn--danger" @click="handleRemove(row as Domain)">
                    <el-icon :size="13"><Delete /></el-icon>
                  </button>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无域名数据，点击右上角「添加域名」配置第一个域名" />
          </template>
        </el-table>
      </div>
    </section>

    <!-- 新建 / 编辑弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑域名' : '添加域名'"
      width="540px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent>
        <el-form-item label="域名" prop="domain" required>
          <el-input
            v-model="form.domain"
            placeholder="如 dwz.cn"
            clearable
            class="mono"
          />
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="协议" prop="scheme" required>
              <el-select v-model="form.scheme" style="width: 100%">
                <el-option label="https" value="https" />
                <el-option label="http" value="http" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="优先级">
              <el-input-number
                v-model="form.priority"
                :min="0"
                :max="9999"
                controls-position="right"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="备注">
              <el-input v-model="form.name" placeholder="便于识别的备注名" clearable />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="项目">
              <el-input v-model="form.project" placeholder="所属项目" clearable />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="状态">
          <el-radio-group v-model="form.status">
            <el-radio :value="1">启用</el-radio>
            <el-radio :value="0">停用</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">
          {{ isEdit ? '保存修改' : '添加域名' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.head-actions {
  display: flex;
  gap: 10px;
}

.domain {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--dwz-petrol);
}

.row-sub {
  font-size: 12px;
  color: var(--dwz-text-dim);
}

.priority {
  font-weight: 700;
  color: var(--dwz-ink);
  font-variant-numeric: tabular-nums;
}

.clicks {
  font-weight: 700;
  color: var(--dwz-ink);
  font-variant-numeric: tabular-nums;
}

.health-cell {
  display: flex;
  flex-direction: column;
  gap: 3px;
  align-items: center;
}

/* 行内迷你操作按钮 */
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

.mini-btn:hover {
  color: var(--dwz-petrol);
  border-color: var(--dwz-petrol);
  transform: translateY(-1px);
  box-shadow: 0 3px 8px rgba(14, 110, 117, 0.15);
}

.mini-btn--danger:hover {
  color: var(--dwz-bad);
  border-color: var(--dwz-bad);
  box-shadow: 0 3px 8px rgba(220, 38, 38, 0.14);
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
