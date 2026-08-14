<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
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

const loading = ref(false)
const rows = ref<Member[]>([])
const total = ref(0)

const query = reactive({ page: 1, per_page: 20, keyword: '', status: '' as MemberStatus | '' })

async function loadData() {
  loading.value = true
  try {
    const res = await listMembers({ ...query })
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
    ElMessage.error(err instanceof Error ? err.message : '加载注册用户失败')
  } finally {
    loading.value = false
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
    await ElMessageBox.confirm(
      `确定删除注册用户「${row.username}」吗？该操作不可撤销。`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await removeMember(row.id)
    ElMessage.success('注册用户已删除')
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

      <div class="app-table-wrap">
        <el-table v-loading="loading" :data="rows" row-key="id" stripe>
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
                  <button class="mini-btn" @click="handleToggleStatus(row as Member)">
                    <el-icon :size="13"><component :is="row.status === 1 ? CircleClose : CircleCheck" /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="重置密码" placement="top">
                  <button class="mini-btn" @click="handleResetPassword(row as Member)">
                    <el-icon :size="13"><RefreshLeft /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="删除" placement="top">
                  <button class="mini-btn mini-btn--danger" @click="handleRemove(row as Member)">
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
          background
          @current-change="loadData"
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
</style>