<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { Link } from '@element-plus/icons-vue'
import { createShortUrl, updateShortUrl, type ShortUrl, type ShortUrlPayload } from '@/api/short-urls'
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

interface FormState {
  long_url: string
  uid: string
  title: string
  category_id: number | null
  expire_days: number | null
  status: 0 | 1
}

const form = reactive<FormState>({
  long_url: '',
  uid: '',
  title: '',
  category_id: null,
  expire_days: null,
  status: 1
})

/** -1 作为「永久有效」的选择器哨兵值 */
const expireOptions = [
  { label: '永久有效', value: -1 },
  { label: '1 天', value: 1 },
  { label: '7 天', value: 7 },
  { label: '30 天', value: 30 },
  { label: '90 天', value: 90 },
  { label: '180 天', value: 180 },
  { label: '1 年', value: 365 }
]

const expireModel = computed<number>({
  get: () => form.expire_days ?? -1,
  set: (v: number) => {
    form.expire_days = v === -1 ? null : v
  }
})

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
  formRef.value?.clearValidate()
}

watch(visible, (v) => {
  if (!v) return
  if (props.editing) {
    form.long_url = props.editing.long_url
    form.uid = props.editing.uid
    form.title = props.editing.title ?? ''
    form.category_id = props.editing.category_id
    form.expire_days = null
    form.status = props.editing.status === 0 ? 0 : 1
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
      category_id: form.category_id,
      expire_days: form.expire_days
    }
    if (isEdit.value && props.editing) {
      payload.status = form.status
      await updateShortUrl(props.editing.id, payload)
      ElMessage.success('短链已更新')
    } else {
      if (form.uid.trim()) payload.uid = form.uid.trim()
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
            <el-select v-model="expireModel" placeholder="选择有效期" style="width: 100%">
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
</style>
