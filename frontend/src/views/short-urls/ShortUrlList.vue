<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type TableInstance } from 'element-plus'
import {
  Search,
  Plus,
  Delete,
  DocumentCopy,
  EditPen,
  Download,
  Position
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  listShortUrls,
  removeShortUrl,
  batchRemoveShortUrls,
  exportShortUrlsCsv,
  type ShortUrl,
  type ShortUrlQuery
} from '@/api/short-urls'
import {
  SHORT_URL_STATUS,
  URL_CATEGORIES,
  buildShortUrl,
  categoryName
} from '@/utils/constants'
import { copyText } from '@/utils/clipboard'
import ShortUrlForm from './ShortUrlForm.vue'

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
  order: 'desc'
})

const dateRange = ref<[string, string] | null>(null)

// 表单弹窗
const formVisible = ref(false)
const editingRow = ref<ShortUrl | null>(null)

function buildParams(): ShortUrlQuery {
  const params: ShortUrlQuery = { ...query }
  if (dateRange.value) {
    params.date_from = dayjs(dateRange.value[0]).format('YYYY-MM-DD')
    params.date_to = dayjs(dateRange.value[1]).format('YYYY-MM-DD')
  } else {
    params.date_from = ''
    params.date_to = ''
  }
  return params
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
    a.click()
    URL.revokeObjectURL(url)
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
        <el-button
          type="danger"
          plain
          :icon="Delete"
          :disabled="!selected.length"
          @click="handleBatchRemove"
        >
          批量删除<span v-if="selected.length" class="mono">&nbsp;({{ selected.length }})</span>
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
                <el-tooltip content="复制短链" placement="top">
                  <button class="mini-btn" @click="handleCopy(row as ShortUrl)">
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
          <el-table-column label="操作" width="122" fixed="right" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-tooltip content="访问短链" placement="top">
                  <a class="mini-btn" :href="buildShortUrl(row.uid)" target="_blank" rel="noopener">
                    <el-icon :size="13"><Position /></el-icon>
                  </a>
                </el-tooltip>
                <el-tooltip content="编辑" placement="top">
                  <button class="mini-btn" @click="openEdit(row as ShortUrl)">
                    <el-icon :size="13"><EditPen /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="删除" placement="top">
                  <button class="mini-btn mini-btn--danger" @click="handleRemove(row as ShortUrl)">
                    <el-icon :size="13"><Delete /></el-icon>
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
