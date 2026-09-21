<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { Search, Delete, RefreshLeft, CircleCheck, CircleClose } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  listMembers,
  updateMemberStatus,
  resetMemberPassword,
  removeMember,
  type Member,
  type MemberStatus
} from '@/api/members'
import { USER_STATUS } from '@/utils/constants'
import TableSkeleton from '@/components/TableSkeleton.vue'

/** 首屏加载：仅此时展示骨架屏 */
const loading = ref(false)
/** 静默刷新：翻页/筛选时不遮罩，仅顶部细进度条 */
const refreshing = ref(false)
/** 首次加载是否完成，作为骨架屏与内容的切换开关 */
const initialized = ref(false)
const rows = ref<Member[]>([])
const total = ref(0)

/** 骨架行数取 page-size，尺寸与真实表格一致，避免 CLS */
const skeletonRows = computed(() => Math.min(query.per_page || 20, 20))
/** 骨架列宽与真实列（用户 / 邮箱 / 验证 / 状态 / 最近登录 / 注册 / 操作）对齐 */
const skeletonWidths = ['minmax(170px, 1fr)', 'minmax(180px, 1fr)', '80px', '70px', '140px', '140px', '130px']

const showSkeleton = computed(() => loading.value && !initialized.value)
const isBusy = computed(() => loading.value || refreshing.value)

const query = reactive({ page: 1, per_page: 20, keyword: '', status: '' as MemberStatus | '' })

/**
 * 加载注册用户列表。
 * @param silent 静默模式：已有数据时不再遮罩整表
 */
async function loadData(silent = initialized.value) {
  if (silent) {
    refreshing.value = true
  } else {
    loading.value = true
  }
  try {
    const res = await listMembers({ ...query })
    if (Array.isArray(res)) {
      rows.value = res
      total.value = res.length
    } else {
      rows.value = res?.list ?? []
      total.value = res?.total ?? 0
    }
    initialized.value = true
  } catch (err) {
    if (!silent) {
      rows.value = []
      total.value = 0
    }
    ElMessage.error(err instanceof Error ? err.message : '加载注册用户失败')
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function handleToggleStatus(row: Member) {
  const next = row.status === 1 ? 0 : 1
  const label = next === 1 ? '启用' : '禁用'
  try {
    await ElMessageBox.confirm(
      next === 1
        ? `确定启用用户「${row.username}」吗？启用后该用户可正常登录。`
        : `确定禁用用户「${row.username}」吗？禁用后该用户将无法登录。`,
      `${label}确认`,
      { confirmButtonText: label, cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await updateMemberStatus(row.id, next as MemberStatus)
    ElMessage.success(`已${label}`)
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : `${label}失败`)
  }
}

async function handleResetPassword(row: Member) {
  let newPassword: string
  try {
    const { value } = await ElMessageBox.prompt(`为注册用户「${row.username}」设置新密码`, '重置密码', {
      confirmButtonText: '确认重置',
      cancelButtonText: '取消',
      inputType: 'password',
      inputPlaceholder: '至少 6 位',
      inputValidator: (v: string) => (v && v.length >= 6 ? true : '密码至少 6 位')
    })
    newPassword = value
  } catch {
    return
  }
  try {
    await resetMemberPassword(row.id, newPassword)
    ElMessage.success('密码已重置')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '重置密码失败')
  }
}

async function handleRemove(row: Member) {
  try {
    // 删除是级联的（服务端 member pur）：文案必须说清会连带处理其短链与
    // 点击明细，否则管理员以为只删了账号，事后发现短链全 410 会当成故障。
    await ElMessageBox.confirm(
      `确定删除注册用户「${row.username}」吗？该操作不可撤销。` +
        '删除后该用户的短链将全部停止跳转，其点击明细中的访问者信息会被匿名化。',
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await removeMember(row.id)
    ElMessage.success('注册用户已删除，其短链已停用')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '删除失败')
  }
}

onMounted(() => loadData(false))
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          注册用户
          <small>MEMBERS · 公网注册用户</small>
        </h1>
        <p class="app-page__desc">共 {{ total }} 个注册用户</p>
      </div>
    </div>

    <section class="app-card">
      <div class="app-toolbar">
        <el-input
          v-model="query.keyword"
          placeholder="搜索用户名 / 邮箱"
          :prefix-icon="Search"
          clearable
          style="width: 260px"
          @keyup.enter="() => { query.page = 1; loadData() }"
          @clear="() => { query.page = 1; loadData() }"
        />
        <el-select
          v-model="query.status"
          placeholder="状态"
          clearable
          style="width: 130px"
          @change="() => { query.page = 1; loadData() }"
        >
          <el-option label="正常" :value="1" />
          <el-option label="禁用" :value="0" />
        </el-select>
        <el-button type="primary" :icon="Search" @click="query.page = 1; loadData()">查询</el-button>
      </div>

      <div class="app-table-wrap dwz-silent" :class="{ 'is-loading': showSkeleton }">
        <!-- 首屏骨架：行数取 page-size，尺寸与真实表格一致，避免 CLS -->
        <TableSkeleton v-if="showSkeleton" :rows="skeletonRows" :widths="skeletonWidths" />
        <!-- 静默刷新指示：翻页/筛选不再整表遮罩 -->
        <span v-if="refreshing" class="dwz-silent__bar" aria-hidden="true" />
        <el-table v-show="!showSkeleton" :data="rows" row-key="id" stripe>
          <el-table-column label="用户" min-width="190">
            <template #default="{ row }">
              <div class="user-cell">
                <span class="user-cell__avatar">{{ row.username.slice(0, 1).toUpperCase() }}</span>
                <span>
                  <div class="user-cell__name">{{ row.username }}</div>
                  <div class="user-cell__sub mono">#{{ row.id }}</div>
                </span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="email" label="邮箱" min-width="210">
            <template #default="{ row }">
              <span class="mono row-mail">{{ row.email }}</span>
            </template>
          </el-table-column>
          <el-table-column label="邮箱验证" width="96" align="center">
            <template #default="{ row }">
              <el-tag :type="row.email_verified === 1 ? 'success' : 'info'" size="small" effect="light" round>
                {{ row.email_verified === 1 ? '已验证' : '未验证' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="88" align="center">
            <template #default="{ row }">
              <el-tag :type="USER_STATUS[row.status as 0 | 1]?.type ?? 'info'" size="small" round>
                {{ USER_STATUS[row.status as 0 | 1]?.label ?? '未知' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="最近登录" width="170">
            <template #default="{ row }">
              <span v-if="row.last_login_at" class="mono user-cell__sub">
                {{ dayjs(row.last_login_at).format('YYYY-MM-DD HH:mm') }}
              </span>
              <span v-else class="user-cell__sub">从未登录</span>
            </template>
          </el-table-column>
          <el-table-column label="注册时间" width="170">
            <template #default="{ row }">
              <span class="mono user-cell__sub">{{ dayjs(row.created_at).format('YYYY-MM-DD HH:mm') }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150" fixed="right" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-tooltip :content="row.status === 1 ? '禁用' : '启用'" placement="top">
                  <button class="mini-btn" :aria-label="row.status === 1 ? '禁用会员' : '启用会员'" @click="handleToggleStatus(row as Member)">
                    <el-icon :size="13"><component :is="row.status === 1 ? CircleClose : CircleCheck" /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="重置密码" placement="top">
                  <button class="mini-btn" aria-label="重置会员密码" @click="handleResetPassword(row as Member)">
                    <el-icon :size="13"><RefreshLeft /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="删除" placement="top">
                  <button class="mini-btn mini-btn--danger" aria-label="删除会员" @click="handleRemove(row as Member)">
                    <el-icon :size="13"><Delete /></el-icon>
                  </button>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无注册用户" />
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
          :disabled="isBusy"
          background
          @current-change="() => loadData()"
          @size-change="() => { query.page = 1; loadData() }"
        />
      </div>
    </section>
  </div>
</template>

<style scoped>
.user-cell {
  display: flex;
  align-items: center;
  gap: 11px;
}

.user-cell__avatar {
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  display: grid;
  place-items: center;
  border-radius: 9px;
  background: linear-gradient(135deg, #0e6e75, #0a4a50);
  color: #fff;
  font-weight: 800;
  font-size: 14px;
}

.user-cell__name {
  font-weight: 700;
  color: var(--dwz-ink);
  font-size: 13.5px;
}

.user-cell__sub {
  font-size: 12px;
  color: var(--dwz-text-dim);
}

.row-mail {
  font-size: 12.5px;
  color: var(--dwz-text);
}

.mini-btn {
  display: inline-grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border: 1px solid var(--dwz-line);
  border-radius: 7px;
  background: var(--el-bg-color);
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
</style>
