<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { TableInstance } from 'element-plus'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import {
  Search,
  Plus,
  Delete,
  DocumentCopy,
  EditPen,
  Download,
  Upload,
  Position,
  TrendCharts,
  CircleCheck,
  DeleteFilled,
  RefreshLeft
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  listShortUrls,
  removeShortUrl,
  batchRemoveShortUrls,
  batchUpdateShortUrls,
  exportShortUrlsCsv,
  importShortUrls,
  getShortUrlStats,
  checkShortUrl,
  restoreShortUrl,
  type ShortUrl,
  type ShortUrlQuery,
  type LinkStat
} from '@/api/short-urls'
import {
  SHORT_URL_STATUS,
  URL_CATEGORIES,
  buildShortUrl,
  categoryName
} from '@/utils/constants'
import { copyText } from '@/utils/clipboard'
import ShortUrlForm from './ShortUrlForm.vue'
import LinkStatsDialog, { type NormalizedStat } from '@/components/LinkStatsDialog.vue'

const tableRef = ref<TableInstance>()
const loading = ref(false)
const rows = ref<ShortUrl[]>([])
const total = ref(0)
const selected = ref<ShortUrl[]>([])

const query = reactive<ShortUrlQuery>({
  page: 1,
  per_page: 20,
  keyword: '',
  status: '',
  category_id: '',
  date_from: '',
  date_to: '',
  sort: 'created_at',
  order: 'desc',
  include_deleted: 0
})

/** 回收站视图开关：为 1 时只看已删除短链 */
const showTrash = ref(false)

const dateRange = ref<[string, string] | null>(null)

// 表单弹窗
const formVisible = ref(false)
const editingRow = ref<ShortUrl | null>(null)

function buildParams(): ShortUrlQuery {
  const params: ShortUrlQuery = { ...query }
  params.include_deleted = showTrash.value ? 1 : 0
  if (dateRange.value) {
    params.date_from = dayjs(dateRange.value[0]).format('YYYY-MM-DD')
    params.date_to = dayjs(dateRange.value[1]).format('YYYY-MM-DD')
  } else {
    params.date_from = ''
    params.date_to = ''
  }
  return params
}

function toggleTrash() {
  showTrash.value = !showTrash.value
  query.page = 1
  loadData()
}

async function handleRestore(row: ShortUrl) {
  try {
    await ElMessageBox.confirm(`确定恢复短链「${row.uid}」吗？恢复后链接将重新可用。`, '恢复短链', {
      confirmButtonText: '恢复',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }
  try {
    await restoreShortUrl(row.id)
    ElMessage.success('短链已恢复')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '恢复失败')
  }
}

async function loadData() {
  loading.value = true
  try {
    const res = await listShortUrls(buildParams())
    // 兼容后端返回数组或分页包裹
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
    ElMessage.error(err instanceof Error ? err.message : '加载短链列表失败')
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  query.page = 1
  loadData()
}

function handleReset() {
  query.keyword = ''
  query.status = ''
  query.category_id = ''
  dateRange.value = null
  handleSearch()
}

function handlePageChange(page: number) {
  query.page = page
  loadData()
}

function handleSizeChange(size: number) {
  query.per_page = size
  query.page = 1
  loadData()
}

function handleSortChange({ prop, order }: { prop: string | null; order: string | null }) {
  query.sort = prop && ['created_at', 'clicks'].includes(prop) ? prop : 'created_at'
  query.order = order === 'ascending' ? 'asc' : 'desc'
  loadData()
}

/* ---------------- 行操作 ---------------- */

async function handleCopy(row: ShortUrl) {
  try {
    await copyText(buildShortUrl(row.uid))
    ElMessage.success(`已复制：${buildShortUrl(row.uid)}`)
  } catch {
    ElMessage.error('复制失败，请手动复制')
  }
}

function openCreate() {
  editingRow.value = null
  formVisible.value = true
}

function openEdit(row: ShortUrl) {
  editingRow.value = row
  formVisible.value = true
}

const statsVisible = ref(false)
const statsLoading = ref(false)
const stats = ref<LinkStat | null>(null)

// 统计对象归一化（与共享 LinkStatsDialog 组件的 {label, clicks} 结构对齐）
const normalizedStats = computed<NormalizedStat | null>(() => {
  if (!stats.value) return null
  const s = stats.value
  return {
    total: s.total,
    trend: s.trend,
    referrers: s.referrers,
    referrer_types: s.referrer_types ?? [],
    devices: s.devices ?? [],
    browsers: s.browsers ?? [],
    countries: s.countries ?? []
  }
})

async function handleCheck(row: ShortUrl) {
  try {
    const r = await checkShortUrl(row.id)
    if (r.ok) {
      ElMessage.success(`目标可达（HTTP ${r.status}）`)
    } else {
      ElMessage.warning(`目标不可达（HTTP ${r.status || '无响应'}${r.error ? '：' + r.error : ''}）`)
    }
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '检查失败')
  }
}

async function handleStats(row: ShortUrl) {
  statsVisible.value = true
  statsLoading.value = true
  stats.value = null
  try {
    stats.value = await getShortUrlStats(row.id)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '加载统计失败')
  } finally {
    statsLoading.value = false
  }
}

async function handleRemove(row: ShortUrl) {
  try {
    await ElMessageBox.confirm(
      `确定删除短链「${row.uid}」吗？删除后该短码将立即失效。`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await removeShortUrl(row.id)
    ElMessage.success('删除成功')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '删除失败')
  }
}

const batchEditVisible = ref(false)
const batchStatus = ref<number | ''>('')
const batchExpire = ref(0)
const batchUpdating = ref(false)

function openBatchEdit() {
  if (!selected.value.length) {
    ElMessage.warning('请先勾选要操作的短链')
    return
  }
  batchStatus.value = ''
  batchExpire.value = 0
  batchEditVisible.value = true
}

async function submitBatchEdit() {
  const data: { status?: number; expire_days?: number } = {}
  if (batchStatus.value !== '') data.status = Number(batchStatus.value)
  if (batchExpire.value === -1) {
    data.expire_days = 0 // 永久有效：清空有效期
  } else if (batchExpire.value > 0) {
    data.expire_days = batchExpire.value
  }
  if (Object.keys(data).length === 0) {
    ElMessage.warning('请选择要修改的状态或有效期')
    return
  }
  batchUpdating.value = true
  try {
    const r = await batchUpdateShortUrls(selected.value.map((s) => s.id), data)
    ElMessage.success(`已更新 ${r.updated} 条短链`)
    batchEditVisible.value = false
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '批量更新失败')
  } finally {
    batchUpdating.value = false
  }
}

async function handleBatchRemove() {
  if (!selected.value.length) return
  try {
    await ElMessageBox.confirm(
      `确定批量删除选中的 ${selected.value.length} 条短链吗？此操作不可撤销。`,
      '批量删除',
      { confirmButtonText: '全部删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await batchRemoveShortUrls(selected.value.map((r) => r.id))
    ElMessage.success(`已删除 ${selected.value.length} 条短链`)
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '批量删除失败')
  }
}

const exporting = ref(false)

async function handleExport() {
  exporting.value = true
  try {
    const blob = await exportShortUrlsCsv(buildParams())
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `short-urls-${dayjs().format('YYYYMMDD-HHmm')}.csv`
    document.body.appendChild(a)
    a.click()
    a.remove()
    // 延迟释放 blob URL，确保下载已开始（旧浏览器立即 revoke 可能中断下载）
    setTimeout(() => URL.revokeObjectURL(url), 1000)
    ElMessage.success('CSV 导出成功')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '导出失败')
  } finally {
    exporting.value = false
  }
}

function truncateUrl(url: string, len = 46): string {
  return url.length > len ? url.slice(0, len) + '…' : url
}

/* ---------------- 导入 ---------------- */

const importVisible = ref(false)
const importing = ref(false)
const importFormat = ref<'csv' | 'json'>('csv')
const importContent = ref('')
const importResult = ref<{ ok: number; fail: number; errors: string[] } | null>(null)

function openImport() {
  importResult.value = null
  importContent.value = ''
  importVisible.value = true
}

async function handleImport() {
  if (!importContent.value.trim()) {
    ElMessage.warning('请输入要导入的内容')
    return
  }
  importing.value = true
  importResult.value = null
  try {
    const res = await importShortUrls({ format: importFormat.value, content: importContent.value })
    importResult.value = { ok: res.total, fail: res.errors.length, errors: res.errors }
    ElMessage.success(`导入完成：成功 ${res.total} 条，失败 ${res.errors.length} 条`)
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '导入失败')
  } finally {
    importing.value = false
  }
}

function formatExpire(row: ShortUrl): string {
  if (!row.expire_at) return '永久'
  return dayjs(row.expire_at).format('YYYY-MM-DD')
}

onMounted(loadData)
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          短链管理
          <small>SHORT URLS · 创建 / 编辑 / 追踪</small>
        </h1>
        <p class="app-page__desc">共 {{ total.toLocaleString() }} 条短链记录</p>
      </div>
      <div class="head-actions">
        <el-button :icon="Download" :loading="exporting" @click="handleExport">导出 CSV</el-button>
        <el-button :icon="Upload" :loading="importing" @click="openImport">导入</el-button>
        <el-button
          :icon="showTrash ? DeleteFilled : Delete"
          :type="showTrash ? 'warning' : 'default'"
          plain
          @click="toggleTrash"
        >
          {{ showTrash ? '返回列表' : '回收站' }}
        </el-button>
        <el-button
          type="danger"
          plain
          :icon="Delete"
          :disabled="!selected.length"
          @click="handleBatchRemove"
        >
          批量删除<span v-if="selected.length" class="mono">&nbsp;({{ selected.length }})</span>
        </el-button>
        <el-button
          plain
          :icon="EditPen"
          :disabled="!selected.length"
          @click="openBatchEdit"
        >
          批量修改<span v-if="selected.length" class="mono">&nbsp;({{ selected.length }})</span>
        </el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建</el-button>
      </div>
    </div>

    <section class="app-card">
      <!-- 筛选条 -->
      <div class="app-toolbar">
        <el-input
          v-model="query.keyword"
          placeholder="搜索短码 / URL / 标题"
          :prefix-icon="Search"
          clearable
          style="width: 250px"
          @keyup.enter="handleSearch"
          @clear="handleSearch"
        />
        <el-select
          v-model="query.status"
          placeholder="状态"
          clearable
          style="width: 120px"
          @change="handleSearch"
        >
          <el-option label="启用" :value="1" />
          <el-option label="禁用" :value="0" />
          <el-option label="已过期" :value="2" />
        </el-select>
        <el-select
          v-model="query.category_id"
          placeholder="分组"
          clearable
          style="width: 140px"
          @change="handleSearch"
        >
          <el-option v-for="c in URL_CATEGORIES" :key="c.id" :label="c.name" :value="c.id" />
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

      <!-- 表格 -->
      <div class="app-table-wrap">
        <el-table
          ref="tableRef"
          v-loading="loading"
          :data="rows"
          row-key="id"
          stripe
          @selection-change="(val: ShortUrl[]) => (selected = val)"
          @sort-change="handleSortChange"
        >
          <el-table-column type="selection" width="44" />
          <el-table-column label="短码" min-width="150">
            <template #default="{ row }">
              <div class="uid-cell">
                <a :href="row.short_url || buildShortUrl(row.uid)" target="_blank" rel="noopener" class="uid mono">
                  {{ row.uid }}
                </a>
                <el-tooltip v-if="row.has_password" content="此链接已设置访问密码" placement="top">
                  <span class="lock-badge" aria-label="已设置访问密码">🔒</span>
                </el-tooltip>
                <el-tooltip content="复制短链" placement="top">
                  <button class="mini-btn" aria-label="复制短链" @click="handleCopy(row as ShortUrl)">
                    <el-icon :size="13"><DocumentCopy /></el-icon>
                  </button>
                </el-tooltip>
              </div>
              <div v-if="row.title" class="row-sub">{{ row.title }}</div>
            </template>
          </el-table-column>
          <el-table-column label="目标 URL" min-width="280">
            <template #default="{ row }">
              <el-tooltip :content="row.long_url" placement="top" :show-after="400">
                <span class="long-url mono">{{ truncateUrl(row.long_url) }}</span>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column prop="clicks" label="点击数" width="110" sortable="custom" align="right">
            <template #default="{ row }">
              <span class="clicks mono">{{ row.clicks.toLocaleString() }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="92" align="center">
            <template #default="{ row }">
              <el-tag
                :type="SHORT_URL_STATUS[row.status as 0 | 1 | 2]?.type ?? 'info'"
                size="small"
                effect="light"
                round
              >
                {{ SHORT_URL_STATUS[row.status as 0 | 1 | 2]?.label ?? '未知' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="分组" width="104">
            <template #default="{ row }">
              <span class="row-sub">{{ row.category_name || categoryName(row.category_id) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="有效期" width="112">
            <template #default="{ row }">
              <span class="mono expire" :class="{ 'expire--forever': !row.expire_at }">
                {{ formatExpire(row as ShortUrl) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column
            prop="created_at"
            label="创建时间"
            width="170"
            sortable="custom"
          >
            <template #default="{ row }">
              <span class="mono row-sub">{{ dayjs(row.created_at).format('YYYY-MM-DD HH:mm') }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="200" fixed="right" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-tooltip content="访问短链" placement="top">
                  <a class="mini-btn" aria-label="访问短链" :href="buildShortUrl(row.uid)" target="_blank" rel="noopener">
                    <el-icon :size="13"><Position /></el-icon>
                  </a>
                </el-tooltip>
                <el-tooltip content="统计" placement="top">
                  <button class="mini-btn" aria-label="查看统计" @click="handleStats(row as ShortUrl)">
                    <el-icon :size="13"><TrendCharts /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="检查目标" placement="top">
                  <button class="mini-btn" aria-label="检查目标链接" @click="handleCheck(row as ShortUrl)">
                    <el-icon :size="13"><CircleCheck /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="编辑" placement="top">
                  <button class="mini-btn" aria-label="编辑短链" @click="openEdit(row as ShortUrl)">
                    <el-icon :size="13"><EditPen /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="删除" placement="top">
                  <button class="mini-btn mini-btn--danger" aria-label="删除短链" @click="handleRemove(row as ShortUrl)">
                    <el-icon :size="13"><Delete /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip v-if="showTrash" content="恢复" placement="top">
                  <button class="mini-btn" aria-label="恢复短链" @click="handleRestore(row as ShortUrl)">
                    <el-icon :size="13"><RefreshLeft /></el-icon>
                  </button>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无短链数据，点击右上角「新建」创建第一条短链" />
          </template>
        </el-table>
      </div>

      <!-- 分页 -->
      <div class="app-pager">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.per_page"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </section>

    <ShortUrlForm v-model="formVisible" :editing="editingRow" @saved="loadData" />

    <!-- 导入弹窗 -->
    <el-dialog v-model="importVisible" title="批量导入短链" width="580px" :close-on-click-modal="false">
      <el-form label-position="top">
        <el-form-item label="格式">
          <el-radio-group v-model="importFormat">
            <el-radio value="csv">CSV</el-radio>
            <el-radio value="json">JSON</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="内容">
          <el-input
            v-model="importContent"
            type="textarea"
            :rows="10"
            placeholder="CSV 每行一条：url,title,custom,expire_days&#10;示例：https://example.com/a,标题A,,7&#10;&#10;JSON 数组：&#10;[{&quot;url&quot;:&quot;https://example.com/a&quot;,&quot;title&quot;:&quot;标题A&quot;,&quot;expire_days&quot;:7}]"
          />
        </el-form-item>
        <div v-if="importResult" class="import-result">
          <p>成功 {{ importResult.ok }} 条，失败 {{ importResult.fail }} 条</p>
          <ul v-if="importResult.errors.length">
            <li v-for="(e, i) in importResult.errors.slice(0, 20)" :key="i" class="mono">{{ e }}</li>
          </ul>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :loading="importing" @click="handleImport">开始导入</el-button>
      </template>
    </el-dialog>

    <!-- 批量修改弹窗 -->
    <el-dialog v-model="batchEditVisible" title="批量修改短链" width="420px" :close-on-click-modal="false">
      <el-form label-position="top">
        <el-form-item label="状态（不修改请留空）">
          <el-select v-model="batchStatus" placeholder="不修改" clearable style="width: 100%">
            <el-option label="启用" :value="1" />
            <el-option label="禁用" :value="0" />
          </el-select>
        </el-form-item>
        <el-form-item label="有效期（不修改请选 0）">
          <el-select v-model="batchExpire" style="width: 100%">
            <el-option label="不修改" :value="0" />
            <el-option label="永久有效" :value="-1" />
            <el-option label="7 天" :value="7" />
            <el-option label="30 天" :value="30" />
            <el-option label="1 年" :value="365" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchEditVisible = false">取消</el-button>
        <el-button type="primary" :loading="batchUpdating" @click="submitBatchEdit">确定修改</el-button>
      </template>
    </el-dialog>

    <!-- 单链统计弹窗（共享组件） -->
    <LinkStatsDialog v-model="statsVisible" :loading="statsLoading" :stats="normalizedStats" />
  </div>
</template>

<style scoped>
.head-actions {
  display: flex;
  gap: 10px;
}

.uid-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.lock-badge {
  font-size: 12px;
  line-height: 1;
  cursor: default;
}

.uid {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--dwz-petrol);
  text-decoration: none;
  border-bottom: 1px dashed transparent;
  transition: border-color 0.15s ease, color 0.15s ease;
}

.uid:hover {
  color: var(--dwz-amber-deep);
  border-bottom-color: var(--dwz-amber-deep);
}

.long-url {
  font-size: 12.5px;
  color: var(--dwz-text);
  word-break: break-all;
}

.row-sub {
  font-size: 12px;
  color: var(--dwz-text-dim);
}

.clicks {
  font-weight: 700;
  color: var(--dwz-ink);
  font-variant-numeric: tabular-nums;
}

.expire {
  font-size: 12px;
  color: var(--dwz-text);
}

.expire--forever {
  color: var(--dwz-good);
  font-weight: 600;
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
  text-decoration: none;
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
</style>
