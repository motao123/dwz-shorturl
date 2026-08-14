<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { Search, Plus, EditPen, Delete, RefreshLeft } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  listUsers,
  createUser,
  updateUser,
  removeUser,
  resetUserPassword,
  assignUserRoles,
  type AdminUser,
  type UserPayload
} from '@/api/users'
import { listRoles, type Role } from '@/api/roles'
import { USER_STATUS } from '@/utils/constants'

const loading = ref(false)
const rows = ref<AdminUser[]>([])
const total = ref(0)
const allRoles = ref<Role[]>([])

const query = reactive({ page: 1, per_page: 20, keyword: '' })

/* ---------------- 弹窗 ---------------- */

const dialogVisible = ref(false)
const submitting = ref(false)
const editingRow = ref<AdminUser | null>(null)
const isEdit = ref(false)

const formRef = ref()
const form = reactive({
  username: '',
  email: '',
  display_name: '',
  password: '',
  status: 1 as 0 | 1,
  role_ids: [] as number[]
})

const rules = reactive({
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { pattern: /^[a-zA-Z][a-zA-Z0-9_]{1,31}$/, message: '字母开头，2-32 位字母数字下划线', trigger: 'blur' }
  ],
  email: [
    { required: true, message: '请输入邮箱', trigger: 'blur' },
    { type: 'email' as const, message: '邮箱格式不正确', trigger: 'blur' }
  ],
  password: [
    {
      validator: (_: unknown, value: string, cb: (e?: Error) => void) => {
        if (!isEdit.value && !value) return cb(new Error('请设置初始密码'))
        if (value && value.length < 6) return cb(new Error('密码至少 6 位'))
        cb()
      },
      trigger: 'blur'
    }
  ]
})

async function loadRoles() {
  try {
    const res = await listRoles()
    allRoles.value = Array.isArray(res) ? res : ((res as unknown as { list: Role[] })?.list ?? [])
  } catch {
    allRoles.value = []
  }
}

async function loadData() {
  loading.value = true
  try {
    const res = await listUsers({ ...query })
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
    ElMessage.error(err instanceof Error ? err.message : '加载用户列表失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingRow.value = null
  isEdit.value = false
  Object.assign(form, { username: '', email: '', display_name: '', password: '', status: 1, role_ids: [] })
  dialogVisible.value = true
}

function openEdit(row: AdminUser) {
  editingRow.value = row
  isEdit.value = true
  Object.assign(form, {
    username: row.username,
    email: row.email,
    display_name: row.display_name ?? '',
    password: '',
    status: row.status,
    role_ids: row.role_ids ?? []
  })
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
    let userId: number
    if (isEdit.value && editingRow.value) {
      const payload: Partial<UserPayload> = {
        email: form.email,
        display_name: form.display_name || undefined,
        status: form.status
      }
      if (form.password) payload.password = form.password
      await updateUser(editingRow.value.id, payload)
      userId = editingRow.value.id
    } else {
      const created = await createUser({
        username: form.username.trim(),
        email: form.email.trim(),
        password: form.password,
        display_name: form.display_name || undefined,
        status: form.status
      })
      userId = created?.id ?? 0
    }

    // 同步角色；新建用户时角色绑定失败则回滚删除已创建的用户，避免孤儿账号
    if (userId) {
      try {
        await assignUserRoles(userId, form.role_ids)
      } catch (roleErr) {
        if (!isEdit.value && userId) {
          try {
            await removeUser(userId)
          } catch {
            /* 回滚失败仅提示，不掩盖原始错误 */
          }
        }
        throw roleErr
      }
    }

    ElMessage.success(isEdit.value ? '用户已更新' : '用户创建成功')
    dialogVisible.value = false
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    submitting.value = false
  }
}

async function handleResetPassword(row: AdminUser) {
  let newPassword: string
  try {
    const { value } = await ElMessageBox.prompt(`为用户「${row.username}」设置新密码`, '重置密码', {
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
    await resetUserPassword(row.id, newPassword)
    ElMessage.success('密码已重置')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '重置密码失败')
  }
}

async function handleRemove(row: AdminUser) {
  try {
    await ElMessageBox.confirm(
      `确定删除用户「${row.username}」吗？该用户将立即失去后台访问权限。`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await removeUser(row.id)
    ElMessage.success('用户已删除')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '删除失败')
  }
}

function roleDisplayName(name: string): string {
  const map: Record<string, string> = {
    super_admin: '超级管理员',
    admin: '管理员',
    operator: '运营',
    viewer: '只读'
  }
  return allRoles.value.find((r) => r.name === name)?.display_name || map[name] || name
}

onMounted(() => {
  loadRoles()
  loadData()
})
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          用户管理
          <small>USERS · 后台管理员账户</small>
        </h1>
        <p class="app-page__desc">共 {{ total }} 个管理员账户</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建用户</el-button>
    </div>

    <section class="app-card">
      <div class="app-toolbar">
        <el-input
          v-model="query.keyword"
          placeholder="搜索用户名 / 邮箱 / 昵称"
          :prefix-icon="Search"
          clearable
          style="width: 260px"
          @keyup.enter="() => { query.page = 1; loadData() }"
          @clear="() => { query.page = 1; loadData() }"
        />
        <el-button type="primary" :icon="Search" @click="query.page = 1; loadData()">查询</el-button>
      </div>

      <div class="app-table-wrap">
        <el-table v-loading="loading" :data="rows" row-key="id" stripe>
          <el-table-column label="用户" min-width="190">
            <template #default="{ row }">
              <div class="user-cell">
                <span class="user-cell__avatar">{{ (row.display_name || row.username).slice(0, 1).toUpperCase() }}</span>
                <span>
                  <div class="user-cell__name">{{ row.username }}</div>
                  <div class="user-cell__sub">{{ row.display_name || '—' }}</div>
                </span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="email" label="邮箱" min-width="210">
            <template #default="{ row }">
              <span class="mono row-mail">{{ row.email }}</span>
            </template>
          </el-table-column>
          <el-table-column label="角色" min-width="180">
            <template #default="{ row }">
              <template v-if="row.roles?.length">
                <el-tag
                  v-for="r in row.roles"
                  :key="r"
                  size="small"
                  effect="plain"
                  round
                  class="role-tag"
                  :class="{ 'role-tag--super': r === 'super_admin' }"
                >
                  {{ roleDisplayName(r) }}
                </el-tag>
              </template>
              <span v-else class="user-cell__sub">未分配</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="88" align="center">
            <template #default="{ row }">
              <el-tag :type="USER_STATUS[row.status as 0 | 1]?.type ?? 'info'" size="small" round>
                {{ USER_STATUS[row.status as 0 | 1]?.label ?? '未知' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="最近登录" width="165">
            <template #default="{ row }">
              <span v-if="row.last_login_at" class="mono user-cell__sub">
                {{ dayjs(row.last_login_at).format('YYYY-MM-DD HH:mm') }}
              </span>
              <span v-else class="user-cell__sub">从未登录</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="168" fixed="right" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-tooltip content="编辑" placement="top">
                  <button class="mini-btn" @click="openEdit(row as AdminUser)">
                    <el-icon :size="13"><EditPen /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="重置密码" placement="top">
                  <button class="mini-btn" @click="handleResetPassword(row as AdminUser)">
                    <el-icon :size="13"><RefreshLeft /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="删除" placement="top">
                  <button class="mini-btn mini-btn--danger" @click="handleRemove(row as AdminUser)">
                    <el-icon :size="13"><Delete /></el-icon>
                  </button>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无用户数据" />
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

    <!-- 新建 / 编辑弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? `编辑用户 · ${form.username}` : '新建用户'"
      width="540px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="用户名" prop="username">
              <el-input v-model="form.username" :disabled="isEdit" placeholder="如 zhangsan" class="mono" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="显示名称">
              <el-input v-model="form.display_name" placeholder="如 张三" clearable />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="user@example.com" clearable />
        </el-form-item>

        <el-form-item :label="isEdit ? '新密码（留空则不修改）' : '初始密码'" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 6 位" />
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="状态">
              <el-radio-group v-model="form.status">
                <el-radio :value="1">正常</el-radio>
                <el-radio :value="0">禁用</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="角色">
              <el-select v-model="form.role_ids" multiple placeholder="选择角色" style="width: 100%">
                <el-option
                  v-for="r in allRoles"
                  :key="r.id"
                  :label="r.display_name"
                  :value="r.id"
                />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">
          {{ isEdit ? '保存修改' : '创建用户' }}
        </el-button>
      </template>
    </el-dialog>
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

.role-tag {
  margin-right: 5px;
}

.role-tag--super {
  color: var(--dwz-amber-deep);
  border-color: rgba(245, 166, 35, 0.4);
  background: rgba(245, 166, 35, 0.08);
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

:deep(.el-form-item__label) {
  font-weight: 700;
  color: var(--dwz-ink);
}
</style>
