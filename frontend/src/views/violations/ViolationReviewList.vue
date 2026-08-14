<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { Search, Delete, CircleCheck } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  listViolations,
  markViolationReviewed,
  removeViolation,
  type ViolationReview,
  type Reviewed
} from '@/api/violations'

const loading = ref(false)
const rows = ref<ViolationReview[]>([])
const total = ref(0)

const query = reactive({ page: 1, per_page: 20, keyword: '', reviewed: '' as Reviewed | '' })

async function loadData() {
  loading.value = true
  try {
    const res = await listViolations({ ...query })
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
    ElMessage.error(err instanceof Error ? err.message : '加载违规记录失败')
  } finally {
    loading.value = false
  }
}

async function handleReview(row: ViolationReview) {
  let note: string
  try {
    const { value } = await ElMessageBox.prompt('归档该记录，可填写处理备注', '标记已审', {
      confirmButtonText: '确认归档',
      cancelButtonText: '取消',
      inputPlaceholder: '备注（可选）',
      inputValue: row.note ?? ''
    })
    note = value
  } catch {
    return
  }
  try {
    await markViolationReviewed(row.id, note)
    ElMessage.success('已归档')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '操作失败')
  }
}

async function handleRemove(row: ViolationReview) {
  try {
    await ElMessageBox.confirm('确定删除该条违规记录吗？该操作不可撤销。', '删除确认', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }
  try {
    await removeViolation(row.id)
    ElMessage.success('已删除')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '删除失败')
  }
}

onMounted(loadData)
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          违规审核
          <small>VIOLATIONS · 被拦截链接的待审清单</small>
        </h1>
        <p class="app-page__desc">共 {{ total }} 条记录</p>
      </div>
    </div>

    <section class="app-card">
      <div class="app-toolbar">
        <el-input
          v-model="query.keyword"
          placeholder="搜索链接 / 原因"
          :prefix-icon="Search"
          clearable
          style="width: 260px"
          @keyup.enter="() => { query.page = 1; loadData() }"
          @clear="() => { query.page = 1; loadData() }"
        />
        <el-select
          v-model="query.reviewed"
          placeholder="状态"
          clearable
          style="width: 130px"
          @change="() => { query.page = 1; loadData() }"
        >
          <el-option label="待审核" :value="0" />
          <el-option label="已归档" :value="1" />
        </el-select>
        <el-button type="primary" :icon="Search" @click="query.page = 1; loadData()">查询</el-button>
      </div>

      <div class="app-table-wrap">
        <el-table v-loading="loading" :data="rows" row-key="id" stripe>
          <el-table-column label="目标地址" min-width="320">
            <template #default="{ row }">
              <span class="v-url">{{ row.url }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="reason" label="命中原因" min-width="150">
            <template #default="{ row }">
              <el-tag size="small" effect="plain" round>{{ row.reason || '—' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="来源" width="90" align="center">
            <template #default="{ row }">
              <el-tag
                size="small"
                effect="plain"
                round
                :type="row.source === 'batch' ? 'success' : 'info'"
              >
                {{ row.source }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="ip" label="IP" width="130">
            <template #default="{ row }">
              <span class="mono">{{ row.ip || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90" align="center">
            <template #default="{ row }">
              <el-tag :type="row.reviewed === 1 ? 'success' : 'danger'" size="small" round>
                {{ row.reviewed === 1 ? '已归档' : '待审核' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="备注" min-width="120">
            <template #default="{ row }">
              <span class="row-sub">{{ row.note || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="时间" width="170">
            <template #default="{ row }">
              <span class="mono row-sub">{{ dayjs(row.created_at).format('YYYY-MM-DD HH:mm') }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-tooltip content="标记已审" placement="top">
                  <button class="mini-btn" :disabled="row.reviewed === 1" @click="handleReview(row as ViolationReview)">
                    <el-icon :size="13"><CircleCheck /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="删除" placement="top">
                  <button class="mini-btn mini-btn--danger" @click="handleRemove(row as ViolationReview)">
                    <el-icon :size="13"><Delete /></el-icon>
                  </button>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无违规记录" />
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
  </div>
</template>

<style scoped>
.v-url {
  font-size: 12.5px;
  color: var(--dwz-text);
  overflow-wrap: anywhere;
}

.row-sub {
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

.mini-btn:hover:not(:disabled) {
  color: var(--dwz-petrol);
  border-color: var(--dwz-petrol);
  transform: translateY(-1px);
  box-shadow: 0 3px 8px rgba(14, 110, 117, 0.15);
}

.mini-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.mini-btn--danger:hover:not(:disabled) {
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