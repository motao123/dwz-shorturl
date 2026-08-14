<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import type { FormRules, FormInstance } from 'element-plus'
import { Link } from '@element-plus/icons-vue'
import { createShortUrl, updateShortUrl, type ShortUrl, type ShortUrlPayload } from '@/api/short-urls'
import { getActiveDomains, type ActiveDomain } from '@/api/domains'
import { URL_CATEGORIES } from '@/utils/constants'

interface Props {
  modelValue: boolean
  /** 传入则为编辑模式 */
  editing?: ShortUrl | null
}

const props = withDefaults(defineProps<Props>(), { editing: null })
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  saved: []
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v)
})

const isEdit = computed(() => Boolean(props.editing))
const formRef = ref<FormInstance>()
const submitting = ref(false)

const activeDomains = ref<ActiveDomain[]>([])

async function loadActiveDomains() {
  try {
    const res = await getActiveDomains()
    activeDomains.value = Array.isArray(res) ? res : []
  } catch {
    activeDomains.value = []
  }
}

onMounted(loadActiveDomains)

interface FormState {
  long_url: string
  uid: string
  title: string
  category_id: number | null
  expire_days: number | null
  status: 0 | 1
  domain_id: number | null
  password: string
}

const form = reactive<FormState>({
  long_url: '',
  uid: '',
  title: '',
  category_id: null,
  expire_days: null,
  status: 1,
  domain_id: null,
  password: ''
})

/** 编辑模式的密码策略：keep=不修改 set=设置新密码 clear=清除密码 */
const passwordMode = ref<'keep' | 'set' | 'clear'>('keep')

/** -1 作为「永久有效」的选择器哨兵值；-2 仅编辑模式使用，表示「不修改」 */
const EXPIRE_UNCHANGED = -2
const expireOptions = [
  { label: '永久有效', value: -1 },
  { label: '1 天', value: 1 },
  { label: '7 天', value: 7 },
  { label: '30 天', value: 30 },
  { label: '90 天', value: 90 },
  { label: '180 天', value: 180 },
  { label: '1 年', value: 365 }
]

/** 有效期下拉当前选中值（与 form.expire_days 分离，-2 不修改 不写入 form） */
const expireSel = ref(-1)

/** 编辑模式是否改动过有效期；未改动时提交 payload 省略 expire_days，避免把原有有效期清成永久 */
const expireChanged = ref(false)

function setExpireSel(v: number) {
  if (v === EXPIRE_UNCHANGED) return // 保持 form.expire_days 原值，提交时省略该字段
  expireChanged.value = true
  expireSel.value = v
  form.expire_days = v === -1 ? null : v
}

const urlValidator = (_: unknown, value: string, callback: (err?: Error) => void) => {
  if (!value) return callback(new Error('请输入目标 URL'))
  try {
    const u = new URL(value)
    if (!['http:', 'https:'].includes(u.protocol)) {
      return callback(new Error('仅支持 http / https 链接'))
    }
  } catch {
    return callback(new Error('URL 格式不正确'))
  }
  callback()
}

const rules: FormRules = {
  long_url: [{ required: true, validator: urlValidator, trigger: 'blur' }],
  uid: [
    {
      pattern: /^[a-zA-Z0-9_-]{3,16}$/,
      message: '短码为 3-16 位字母、数字、下划线或连字符',
      trigger: 'blur'
    }
  ],
  title: [{ max: 255, message: '标题不能超过 255 个字符', trigger: 'blur' }]
}

function resetForm() {
  form.long_url = ''
  form.uid = ''
  form.title = ''
  form.category_id = null
  form.expire_days = null
  form.status = 1
  form.domain_id = null
  form.password = ''
  passwordMode.value = 'keep'
  expireSel.value = -1
  expireChanged.value = false
  formRef.value?.clearValidate()
}

watch(visible, (v) => {
  if (!v) return
  if (props.editing) {
    form.long_url = props.editing.long_url
    form.uid = props.editing.uid
    form.title = props.editing.title ?? ''
    form.category_id = props.editing.category_id
    // 回填当前有效期：永久 → 永久；剩余天数能精确匹配选项 → 该天数；否则 → 不修改
    expireChanged.value = false
    expireSel.value = -1
    form.expire_days = null
    if (props.editing.expire_at) {
      const remainingDays = Math.round((new Date(props.editing.expire_at).getTime() - Date.now()) / 86400000)
      const matched = expireOptions.find((o) => o.value !== -1 && o.value === remainingDays)
      if (matched) {
        expireSel.value = matched.value
        form.expire_days = matched.value
        expireChanged.value = true
      } else {
        expireSel.value = EXPIRE_UNCHANGED
      }
    }
    // status: 仅 1=启用，其余（禁用/已过期）都按不可用展示为「禁用」，避免编辑过期链接被静默改为启用
    form.status = props.editing.status === 1 ? 1 : 0
    form.domain_id = props.editing.domain_id ?? null
    // 密码：默认「不修改」，仅当用户显式选择设置/清除时才随提交发送
    form.password = ''
    passwordMode.value = props.editing.has_password ? 'keep' : 'keep'
  } else {
    resetForm()
  }
})

async function handleSubmit() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  submitting.value = true
  try {
    const payload: ShortUrlPayload = {
      long_url: form.long_url.trim(),
      title: form.title.trim() || undefined,
      category_id: form.category_id
    }
    // 编辑模式未改动有效期时省略 expire_days，后端保持原有效期不变
    if (!(isEdit.value && !expireChanged.value)) {
      payload.expire_days = form.expire_days
    }
    if (isEdit.value) {
      // 密码：仅当用户显式选择「设置/清除」时才发送，避免误清
      if (passwordMode.value === 'set' && form.password) {
        payload.password = form.password
      } else if (passwordMode.value === 'clear') {
        payload.password = ''
      }
    } else if (form.password) {
      payload.password = form.password
    }
    if (isEdit.value && props.editing) {
      payload.status = form.status
      await updateShortUrl(props.editing.id, payload)
      ElMessage.success('短链已更新')
    } else {
      if (form.uid.trim()) payload.uid = form.uid.trim()
      if (form.domain_id) payload.domain_id = form.domain_id
      await createShortUrl(payload)
      ElMessage.success('短链创建成功')
    }
    visible.value = false
    emit('saved')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : (isEdit.value ? '更新失败' : '创建失败'))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="isEdit ? '编辑短链' : '新建短链'"
    width="560px"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent>
      <el-form-item label="目标 URL" prop="long_url" required>
        <el-input
          v-model="form.long_url"
          placeholder="https://example.com/very/long/path"
          clearable
          :maxlength="2048"
        >
          <template #prefix><el-icon><Link /></el-icon></template>
        </el-input>
      </el-form-item>

      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item prop="uid">
            <template #label>
              自定义短码
              <span class="hint mono">可选</span>
            </template>
            <el-input
              v-model="form.uid"
              :disabled="isEdit"
              placeholder="如 my-link"
              clearable
              :maxlength="16"
              class="mono"
            />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="有效期">
            <el-select v-model="expireSel" placeholder="选择有效期" style="width: 100%" @change="setExpireSel">
              <el-option v-if="isEdit" label="不修改（保留当前有效期）" :value="EXPIRE_UNCHANGED" />
              <el-option
                v-for="opt in expireOptions"
                :key="String(opt.value)"
                :label="opt.label"
                :value="opt.value"
              />
            </el-select>
          </el-form-item>
        </el-col>
      </el-row>

      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item prop="title">
            <template #label>
              标题
              <span class="hint mono">可选</span>
            </template>
            <el-input v-model="form.title" placeholder="便于识别的备注名" clearable />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="分组">
            <el-select v-model="form.category_id" placeholder="选择分组" clearable style="width: 100%">
              <el-option v-for="c in URL_CATEGORIES" :key="c.id" :label="c.name" :value="c.id" />
            </el-select>
          </el-form-item>
        </el-col>
      </el-row>

      <el-form-item label="短链域名">
        <el-select
          v-model="form.domain_id"
          placeholder="默认域名"
          clearable
          style="width: 100%"
          :disabled="!activeDomains.length"
        >
          <el-option
            v-for="d in activeDomains"
            :key="d.id"
            :label="`${d.scheme}://${d.domain}`"
            :value="d.id"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="访问密码">
        <div v-if="isEdit" class="pw-mode">
          <el-radio-group v-model="passwordMode" class="pw-mode__radios">
            <el-radio value="keep">{{ editing?.has_password ? '保留现有密码' : '不设置' }}</el-radio>
            <el-radio value="set">设置新密码</el-radio>
            <el-radio v-if="editing?.has_password" value="clear">清除密码</el-radio>
          </el-radio-group>
          <el-input
            v-if="passwordMode === 'set'"
            v-model="form.password"
            type="password"
            show-password
            placeholder="可选，访问此链接需输入密码"
            :maxlength="72"
            clearable
            class="pw-mode__input"
          />
        </div>
        <el-input
          v-else
          v-model="form.password"
          type="password"
          show-password
          placeholder="可选，访问此链接需输入密码"
          :maxlength="72"
          clearable
        />
      </el-form-item>

      <el-form-item v-if="isEdit" label="状态">
        <el-radio-group v-model="form.status">
          <el-radio :value="1">启用</el-radio>
          <el-radio :value="0">禁用</el-radio>
        </el-radio-group>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">
        {{ isEdit ? '保存修改' : '创建短链' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.hint {
  margin-left: 6px;
  font-size: 10px;
  letter-spacing: 0.1em;
  color: var(--dwz-text-dim);
  font-weight: 400;
}

:deep(.el-form-item__label) {
  font-weight: 700;
  color: var(--dwz-ink);
  padding-bottom: 4px;
}

.pw-mode {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.pw-mode__radios {
  width: 100%;
}

.pw-mode__input {
  width: 100%;
}
</style>
