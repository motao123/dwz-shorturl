<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, EditPen, Delete, Key, Lock } from '@element-plus/icons-vue'
import {
  listRoles,
  createRole,
  updateRole,
  removeRole,
  setRolePermissions,
  getAllPermissions,
  type Role,
  type Permission
} from '@/api/roles'

const loading = ref(false)
const rows = ref<Role[]>([])
const allPermissions = ref<Permission[]>([])

/* ---------------- 角色编辑弹窗 ---------------- */

const dialogVisible = ref(false)
const submitting = ref(false)
const isEdit = ref(false)
const editingRow = ref<Role | null>(null)
const formRef = ref()

const form = reactive({ name: '', display_name: '', description: '' })

const rules = {
  name: [
    { required: true, message: '请输入角色标识', trigger: 'blur' },
    { pattern: /^[a-z][a-z0-9_]{1,31}$/, message: '小写字母开头，2-32 位小写字母数字下划线', trigger: 'blur' }
  ],
  display_name: [{ required: true, message: '请输入角色名称', trigger: 'blur' }]
}

/* ---------------- 权限分配弹窗 ---------------- */

const permDialogVisible = ref(false)
const permSaving = ref(false)
const permRole = ref<Role | null>(null)
const treeRef = ref()
const checkedPerms = ref<string[]>([])

interface PermNode {
  id: string
  label: string
  children?: PermNode[]
}

const RESOURCE_LABELS: Record<string, string> = {
  short_urls: '短链管理',
  stats: '统计分析',
  users: '用户管理',
  roles: '角色权限',
  configs: '系统配置',
  audit: '审计日志',
  api_keys: 'API 密钥'
}

const ACTION_LABELS: Record<string, string> = {
  create: '创建',
  read: '查看',
  update: '编辑',
  delete: '删除',
  export: '导出',
  assign_roles: '分配角色',
  revoke: '吊销'
}

const permTree = computed<PermNode[]>(() => {
  const grouped = new Map<string, Permission[]>()
  for (const p of allPermissions.value) {
    const arr = grouped.get(p.resource) ?? []
    arr.push(p)
    grouped.set(p.resource, arr)
  }
  return [...grouped.entries()].map(([resource, perms]) => ({
    id: resource,
    label: RESOURCE_LABELS[resource] ?? resource,
    children: perms.map((p) => ({
      id: `${p.resource}.${p.action}`,
      label: ACTION_LABELS[p.action] ?? p.action
    }))
  }))
})

const treeProps = { children: 'children', label: 'label' }

async function loadData() {
  loading.value = true
  try {
    const res = await listRoles()
    rows.value = Array.isArray(res) ? res : ((res as unknown as { list: Role[] })?.list ?? [])
  } catch (err) {
    rows.value = []
    ElMessage.error(err instanceof Error ? err.message : '加载角色列表失败')
  } finally {
    loading.value = false
  }
}

async function loadPermissions() {
  try {
    const res = await getAllPermissions()
    allPermissions.value = Array.isArray(res) ? res : ((res as unknown as { list: Permission[] })?.list ?? [])
  } catch {
    allPermissions.value = []
  }
}

function openCreate() {
  isEdit.value = false
  editingRow.value = null
  Object.assign(form, { name: '', display_name: '', description: '' })
  dialogVisible.value = true
}

function openEdit(row: Role) {
  isEdit.value = true
  editingRow.value = row
  Object.assign(form, {
    name: row.name,
    display_name: row.display_name,
    description: row.description ?? ''
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
    if (isEdit.value && editingRow.value) {
      await updateRole(editingRow.value.id, {
        display_name: form.display_name,
        description: form.description || undefined
      })
      ElMessage.success('角色已更新')
    } else {
      await createRole({
        name: form.name.trim(),
        display_name: form.display_name.trim(),
        description: form.description || undefined
      })
      ElMessage.success('角色创建成功')
    }
    dialogVisible.value = false
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    submitting.value = false
  }
}

async function handleRemove(row: Role) {
  if (row.is_system === 1) {
    ElMessage.warning('系统内置角色不可删除')
    return
  }
  try {
    await ElMessageBox.confirm(
      `确定删除角色「${row.display_name}」吗？已关联该角色的用户将失去对应权限。`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await removeRole(row.id)
    ElMessage.success('角色已删除')
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '删除失败')
  }
}

function openPermDialog(row: Role) {
  permRole.value = row
  // 仅回填叶子权限点，资源父节点由 el-tree 自动推导半选 / 全选
  checkedPerms.value = (Array.isArray(row.permissions) ? row.permissions : []).filter((id) =>
    id.includes('.')
  )
  permDialogVisible.value = true
}

async function handlePermSave() {
  if (!permRole.value || !treeRef.value) return
  permSaving.value = true
  try {
    // 仅取叶子节点（具体权限点），父级资源节点由 el-tree 自动半选
    const leaves: string[] = treeRef.value.getCheckedKeys(true)
    await setRolePermissions(permRole.value.id, leaves)
    ElMessage.success('权限已保存')
    permDialogVisible.value = false
    loadData()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '保存权限失败')
  } finally {
    permSaving.value = false
  }
}

onMounted(() => {
  loadData()
  loadPermissions()
})
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          角色管理
          <small>ROLES · RBAC 权限模型</small>
        </h1>
        <p class="app-page__desc">通过角色聚合权限点，再为用户分配角色</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建角色</el-button>
    </div>

    <section class="app-card">
      <div class="app-table-wrap" style="padding-top: 16px">
        <el-table v-loading="loading" :data="rows" row-key="id" stripe>
          <el-table-column label="角色" min-width="220">
            <template #default="{ row }">
              <div class="role-cell">
                <span class="role-cell__badge mono">{{ row.name }}</span>
                <el-tag v-if="row.is_system === 1" size="small" effect="dark" round class="role-cell__sys">
                  <el-icon style="margin-right: 3px"><Lock /></el-icon>系统内置
                </el-tag>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="display_name" label="名称" min-width="140">
            <template #default="{ row }">
              <span class="role-name">{{ row.display_name }}</span>
            </template>
          </el-table-column>
          <el-table-column label="描述" min-width="260">
            <template #default="{ row }">
              <span class="role-desc">{{ row.description || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="权限数" width="100" align="center">
            <template #default="{ row }">
              <span class="mono perm-count">{{ Array.isArray(row.permissions) ? row.permissions.length : '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="168" fixed="right" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-tooltip content="分配权限" placement="top">
                  <button class="mini-btn" @click="openPermDialog(row as Role)">
                    <el-icon :size="13"><Key /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip content="编辑" placement="top">
                  <button class="mini-btn" @click="openEdit(row as Role)">
                    <el-icon :size="13"><EditPen /></el-icon>
                  </button>
                </el-tooltip>
                <el-tooltip
                  :content="row.is_system === 1 ? '系统内置角色不可删除' : '删除'"
                  placement="top"
                >
                  <span>
                    <button
                      class="mini-btn mini-btn--danger"
                      :disabled="row.is_system === 1"
                      @click="handleRemove(row as Role)"
                    >
                      <el-icon :size="13"><Delete /></el-icon>
                    </button>
                  </span>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="暂无角色数据" />
          </template>
        </el-table>
      </div>
    </section>

    <!-- 角色编辑弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? `编辑角色 · ${form.display_name || form.name}` : '新建角色'"
      width="480px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="角色标识" prop="name">
          <el-input
            v-model="form.name"
            :disabled="isEdit"
            placeholder="如 operator"
            class="mono"
          />
          <div class="form-hint">唯一标识，创建后不可修改</div>
        </el-form-item>
        <el-form-item label="角色名称" prop="display_name">
          <el-input v-model="form.display_name" placeholder="如 内容运营" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="3"
            maxlength="255"
            show-word-limit
            placeholder="角色职责说明"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">
          {{ isEdit ? '保存修改' : '创建角色' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 权限分配弹窗 -->
    <el-dialog
      v-model="permDialogVisible"
      :title="`分配权限 · ${permRole?.display_name ?? ''}`"
      width="520px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <div v-if="permRole?.is_system === 1" class="perm-warn">
        <el-icon><Lock /></el-icon> 系统内置角色的权限为默认全集，修改可能影响系统行为
      </div>
      <el-tree
        ref="treeRef"
        :data="permTree"
        :props="treeProps"
        node-key="id"
        show-checkbox
        default-expand-all
        :default-checked-keys="checkedPerms"
        class="perm-tree"
      />
      <template #footer>
        <el-button @click="permDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="permSaving" @click="handlePermSave">保存权限</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.role-cell {
  display: flex;
  align-items: center;
  gap: 9px;
}

.role-cell__badge {
  padding: 3px 9px;
  border-radius: 6px;
  background: rgba(14, 110, 117, 0.08);
  border: 1px solid rgba(14, 110, 117, 0.18);
  color: var(--dwz-petrol-strong);
  font-size: 12px;
  font-weight: 600;
}

.role-cell__sys {
  background: var(--dwz-ink);
  border-color: var(--dwz-ink);
}

.role-name {
  font-weight: 700;
  color: var(--dwz-ink);
}

.role-desc {
  font-size: 12.5px;
  color: var(--dwz-text-dim);
}

.perm-count {
  font-weight: 700;
  color: var(--dwz-amber-deep);
}

.form-hint {
  margin-top: 4px;
  font-size: 11.5px;
  color: var(--dwz-text-dim);
}

.perm-warn {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 14px;
  padding: 9px 12px;
  border-radius: 8px;
  background: rgba(245, 166, 35, 0.1);
  border: 1px solid rgba(245, 166, 35, 0.3);
  color: var(--dwz-amber-deep);
  font-size: 12.5px;
}

.perm-tree {
  max-height: 400px;
  overflow-y: auto;
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
