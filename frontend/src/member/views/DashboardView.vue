<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { Link, Refresh, Delete, CopyDocument, Sunny, Moon, ArrowDown } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { useThemeStore } from '@/stores/theme'
import {
  fetchSession,
  getMyLinks,
  createLink,
  deleteLink,
  logout,
  getLinkStats,
  updateLinkExpiry,
  updateLink,
  batchCreateLinks,
  getSummary,
  exportLinksCsv,
  renewExpiring,
  fetchTitle,
  importLinks,
  sendVerification,
  buildShortUrl,
  type MemberLink,
  type LinkStat,
  type MemberBatchResult,
  type MemberSummary
} from '../api'

const themeStore = useThemeStore()

const router = useRouter()
const member = ref<any>(null)
const loading = ref(false)
const rows = ref<MemberLink[]>([])
const total = ref(0)
const page = ref(1)
const keyword = ref('')
const statusFilter = ref('')
const summary = ref<MemberSummary>({ total_links: 0, total_clicks: 0, month_new: 0 })

const createForm = reactive({ url: '', title: '', custom: '', expire_days: 0 })
const creating = ref(false)

const expireOptions = [
  { label: '永久有效', value: 0 },
  { label: '1 天', value: 1 },
  { label: '7 天', value: 7 },
  { label: '30 天', value: 30 },
  { label: '1 年', value: 365 }
]

async function load() {
  loading.value = true
  try {
    const res = await getMyLinks(page.value, 20, keyword.value.trim(), statusFilter.value)
    rows.value = res.list.map((l) => ({ ...l, short_url: buildShortUrl(l.uid) }))
    total.value = res.total
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  page.value = 1
  load()
}

async function handleCreate() {
  if (!createForm.url.trim()) {
    ElMessage.warning('请输入目标网址')
    return
  }
  creating.value = true
  try {
    const created = await createLink(createForm.url.trim(), createForm.custom.trim(), createForm.expire_days, createForm.title.trim())
    ElMessage.success('短链创建成功')
    createForm.url = ''
    createForm.title = ''
    createForm.custom = ''
    createForm.expire_days = 0
    createdResult.value = created.short_url || buildShortUrl(created.uid)
    createdVisible.value = true
    load()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '创建失败')
  } finally {
    creating.value = false
  }
}

const createdVisible = ref(false)
const createdResult = ref('')

const fetchingTitle = ref(false)
const verifying = ref(false)

async function handleSendVerification() {
  if (!member.value?.email) return
  verifying.value = true
  try {
    await sendVerification(member.value.email)
    ElMessage.success('验证邮件已发送，请查收邮箱')
  } catch (err) {
    ElMessage.warning(err instanceof Error ? err.message : '发送失败')
  } finally {
    verifying.value = false
  }
}

const importVisible = ref(false)
const importText = ref('')
const importing = ref(false)
const importResults = ref<MemberBatchResult[]>([])

function openImport() {
  importText.value = ''
  importResults.value = []
  importVisible.value = true
}

async function submitImport() {
  if (!importText.value.trim()) {
    ElMessage.warning('请输入 CSV 内容')
    return
  }
  importing.value = true
  importResults.value = []
  try {
    importResults.value = await importLinks(importText.value)
    const okCount = importResults.value.filter((r) => !r.error).length
    ElMessage.success(`导入成功 ${okCount} 条，失败 ${importResults.value.length - okCount} 条`)
    load()
    loadSummary()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '导入失败')
  } finally {
    importing.value = false
  }
}

async function handleFetchTitle() {
  const url = createForm.url.trim()
  if (!url) {
    ElMessage.warning('请先输入目标网址')
    return
  }
  fetchingTitle.value = true
  try {
    createForm.title = await fetchTitle(url)
    ElMessage.success('已获取标题')
  } catch (err) {
    ElMessage.warning(err instanceof Error ? err.message : '无法获取标题')
  } finally {
    fetchingTitle.value = false
  }
}

function handleMore(cmd: string, row: MemberLink) {
  if (cmd === 'edit') openEdit(row)
  else if (cmd === 'expiry') openExpiry(row)
  else if (cmd === 'qr') openQr(row.short_url || '')
  else if (cmd === 'stats') openStats(row.uid)
}

async function handleDelete(row: MemberLink) {
  try {
    await ElMessageBox.confirm('确定删除该短链吗？', '删除确认', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }
  try {
    await deleteLink(row.id)
    ElMessage.success('已删除')
    load()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '删除失败')
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.error('复制失败')
  }
}

const statsVisible = ref(false)
const statsLoading = ref(false)
const stats = ref<LinkStat | null>(null)

async function openStats(uid: string) {
  statsVisible.value = true
  statsLoading.value = true
  stats.value = null
  try {
    stats.value = await getLinkStats(uid)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '加载统计失败')
  } finally {
    statsLoading.value = false
  }
}

function maxTrend(): number {
  return stats.value?.trend.reduce((m, t) => Math.max(m, t.clicks), 0) || 1
}

const qrVisible = ref(false)
const qrUrl = ref('')
const qrBox = ref<HTMLElement>()

const expiryVisible = ref(false)
const expiryLink = ref<MemberLink | null>(null)
const expiryDays = ref(0)
const expirySaving = ref(false)

const editVisible = ref(false)
const editLink = ref<MemberLink | null>(null)
const editForm = reactive({ long_url: '', title: '', expire_days: 0 })
const editSaving = ref(false)

async function openEdit(row: MemberLink) {
  editLink.value = row
  editForm.long_url = row.long_url
  editForm.title = row.title || ''
  editForm.expire_days = 0
  editVisible.value = true
}

async function saveEdit() {
  if (!editLink.value) return
  if (!editForm.long_url.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }
  editSaving.value = true
  try {
    await updateLink(editLink.value.id, {
      long_url: editForm.long_url.trim(),
      title: editForm.title.trim(),
      expire_days: editForm.expire_days
    })
    ElMessage.success('已更新')
    editVisible.value = false
    load()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '更新失败')
  } finally {
    editSaving.value = false
  }
}

function openExpiry(row: MemberLink) {
  expiryLink.value = row
  expiryDays.value = 0 // 默认：永久有效
  expiryVisible.value = true
}

async function saveExpiry() {
  if (!expiryLink.value) return
  expirySaving.value = true
  try {
    await updateLinkExpiry(expiryLink.value.id, expiryDays.value)
    ElMessage.success('有效期已更新')
    expiryVisible.value = false
    load()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '更新失败')
  } finally {
    expirySaving.value = false
  }
}

function isExpired(expireAt: string | null): boolean {
  return !!expireAt && new Date(expireAt).getTime() < Date.now()
}

function expiryDaysLeft(expireAt: string | null): number {
  if (!expireAt) return Infinity
  return Math.ceil((new Date(expireAt).getTime() - Date.now()) / 86400000)
}

/** ISO 3166-1 alpha-2 → 中文名 + 旗帜 emoji（地域分布展示） */
const COUNTRY_NAMES: Record<string, string> = {
  CN: '中国', HK: '中国香港', MO: '中国澳门', TW: '中国台湾', US: '美国', JP: '日本',
  KR: '韩国', SG: '新加坡', GB: '英国', DE: '德国', FR: '法国', IT: '意大利',
  ES: '西班牙', RU: '俄罗斯', NL: '荷兰', CA: '加拿大', AU: '澳大利亚', NZ: '新西兰',
  IN: '印度', MY: '马来西亚', TH: '泰国', VN: '越南', ID: '印度尼西亚', PH: '菲律宾',
  BR: '巴西', TR: '土耳其', AE: '阿联酋', SA: '沙特阿拉伯', MX: '墨西哥', CH: '瑞士',
  SE: '瑞典', NO: '挪威', FI: '芬兰', DK: '丹麦', PL: '波兰', UA: '乌克兰',
  IE: '爱尔兰', AT: '奥地利', PT: '葡萄牙', GR: '希腊', CZ: '捷克', IL: '以色列',
  AR: '阿根廷', CL: '智利', ZA: '南非', EG: '埃及', PK: '巴基斯坦', KZ: '哈萨克斯坦'
}

function countryName(code: string): string {
  return COUNTRY_NAMES[code.toUpperCase()] ?? code.toUpperCase()
}

function countryFlag(code: string): string {
  const c = code.toUpperCase()
  if (!/^[A-Z]{2}$/.test(c)) return '🌐'
  const base = 0x1f1e6
  return String.fromCodePoint(base + c.charCodeAt(0) - 65, base + c.charCodeAt(1) - 65)
}

function refTypeIcon(type: string): string {
  switch (type) {
    case '搜索引擎': return '🔍'
    case '社交媒体': return '📣'
    case '直接访问': return '🔗'
    default: return '🌐'
  }
}

function expiryLabel(expireAt: string | null): string {
  const days = expiryDaysLeft(expireAt)
  if (days <= 0) return '已过期'
  if (days <= 7) return `${days} 天后过期`
  return dayjs(expireAt).format('YYYY-MM-DD')
}

function openQr(url: string) {
  qrUrl.value = url
  qrVisible.value = true
  // 动态引入 qrcode 包渲染，避免依赖外部 /assets/qrcode.min.js（生产构建中不存在）
  requestAnimationFrame(async () => {
    const el = qrBox.value
    if (!el) return
    el.innerHTML = ''
    try {
      const QRCode = (await import('qrcode')).default
      const canvas = document.createElement('canvas')
      el.appendChild(canvas)
      await QRCode.toCanvas(canvas, url, { width: 220, margin: 1 })
    } catch {
      el.innerHTML = '<p class="qr-error">二维码生成失败，请使用复制功能</p>'
    }
  })
}

function downloadQr() {
  const el = qrBox.value
  if (!el) return
  const canvas = el.querySelector('canvas')
  const img = el.querySelector('img')
  const href = canvas ? canvas.toDataURL('image/png') : img?.src || ''
  if (!href) {
    ElMessage.error('二维码尚未生成')
    return
  }
  const a = document.createElement('a')
  a.href = href
  a.download = `qrcode-${qrUrl.value.replace(/^https?:\/\//, '').replace(/[^a-z0-9]/gi, '_') || 'qr'}.png`
  document.body.appendChild(a)
  a.click()
  a.remove()
}

async function handleLogout() {
  await logout()
  router.push('/login')
}

const batchVisible = ref(false)
const batchText = ref('')
const batchLoading = ref(false)
const batchResults = ref<MemberBatchResult[]>([])

function openBatch() {
  batchText.value = ''
  batchResults.value = []
  batchVisible.value = true
}

async function submitBatch() {
  const lines = batchText.value.split(/\r?\n/).map((l) => l.trim()).filter(Boolean)
  if (!lines.length) {
    ElMessage.warning('请至少输入一个网址')
    return
  }
  if (lines.length > 100) {
    ElMessage.warning('一次最多 100 条')
    return
  }
  batchLoading.value = true
  batchResults.value = []
  try {
    batchResults.value = await batchCreateLinks(lines)
    const ok = batchResults.value.filter((r) => !r.error).length
    ElMessage.success(`成功 ${ok} 条，失败 ${batchResults.value.length - ok} 条`)
    load()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '批量创建失败')
  } finally {
    batchLoading.value = false
  }
}

async function loadSummary() {
  try {
    summary.value = await getSummary()
  } catch {
    /* ignore */
  }
}

async function handleExport() {
  try {
    await exportLinksCsv()
    ElMessage.success('已导出 CSV')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '导出失败')
  }
}

async function handleRenewExpiring() {
  const pick = await ElMessageBox.confirm('将已过期 / 即将到期（7 天内）的短链统一续期，请选择续期时长。', '一键续期', {
    confirmButtonText: '续期 30 天',
    cancelButtonText: '取消',
    type: 'warning'
  }).catch(() => null)
  if (pick !== 'confirm') return
  try {
    const r = await renewExpiring(30)
    ElMessage.success(r.renewed > 0 ? `已续期 ${r.renewed} 条短链` : '没有需要续期的短链')
    load()
    loadSummary()
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '续期失败')
  }
}

onMounted(async () => {
  const s = await fetchSession()
  member.value = s.member
  load()
  loadSummary()
})
</script>

<template>
  <div class="shell member-shell">
    <header class="topbar">
      <a class="brand" href="/">
        <span class="brand-mark">↗</span>
        <span>短网址会员中心</span>
      </a>
      <div class="user">
        <el-button text :icon="themeStore.dark ? Sunny : Moon" :title="themeStore.dark ? '切换到浅色' : '切换到深色'" @click="themeStore.toggle()" />
        <span class="user-name">{{ member?.username }}</span>
        <a class="home-link" href="/">返回首页</a>
        <el-button text @click="handleLogout">退出</el-button>
      </div>
    </header>

    <main>
      <!-- 邮箱未验证提示 -->
      <div v-if="member && member.email_verified === 0" class="verify-banner">
        <span>您的邮箱尚未验证，验证后可提升账号安全。</span>
        <el-button size="small" type="primary" plain :loading="verifying" @click="handleSendVerification">发送验证邮件</el-button>
      </div>

      <!-- 概览 -->
      <div class="summary-grid">
        <div class="summary-card">
          <span class="summary-num">{{ summary.total_links.toLocaleString() }}</span>
          <span class="summary-label">短链总数</span>
        </div>
        <div class="summary-card">
          <span class="summary-num">{{ summary.total_clicks.toLocaleString() }}</span>
          <span class="summary-label">总点击</span>
        </div>
        <div class="summary-card">
          <span class="summary-num">{{ summary.month_new.toLocaleString() }}</span>
          <span class="summary-label">近 30 天新增</span>
        </div>
      </div>

      <!-- 创建 -->
      <section class="card">
        <h2>创建短链</h2>
        <div class="create-row">
          <el-input v-model="createForm.url" placeholder="粘贴长网址" class="url-input" />
          <el-input v-model="createForm.title" placeholder="标题（可选）" class="title-input" />
          <el-button :loading="fetchingTitle" @click="handleFetchTitle">获取标题</el-button>
          <el-input v-model="createForm.custom" placeholder="自定义短码（可选）" class="custom-input" />
          <el-select v-model="createForm.expire_days" class="expire-select">
            <el-option v-for="o in expireOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
          <el-button type="primary" :icon="Link" :loading="creating" @click="handleCreate">创建</el-button>
          <el-button @click="openBatch">批量创建</el-button>
        </div>
      </section>

      <!-- 我的短链 -->
      <section class="card">
        <div class="card-head">
          <h2>我的短链</h2>
          <div class="card-actions">
            <el-input
              v-model="keyword"
              placeholder="搜索短码 / 目标地址"
              clearable
              class="search-input"
              @keyup.enter="handleSearch"
              @clear="handleSearch"
            />
            <el-select v-model="statusFilter" placeholder="状态" clearable class="status-select" @change="handleSearch">
              <el-option label="有效" value="active" />
              <el-option label="即将过期" value="expiring" />
              <el-option label="已过期" value="expired" />
              <el-option label="已禁用" value="disabled" />
            </el-select>
            <el-button type="primary" @click="handleSearch">搜索</el-button>
            <el-button type="warning" plain @click="handleRenewExpiring">一键续期</el-button>
            <el-button @click="handleExport">导出 CSV</el-button>
            <el-button @click="openImport">导入</el-button>
            <el-button :icon="Refresh" circle @click="load" />
          </div>
        </div>
        <el-table v-loading="loading" :data="rows" stripe>
          <el-table-column label="短链" min-width="200">
            <template #default="{ row }">
              <a :href="row.short_url" target="_blank" rel="noopener" class="short-link">{{ row.short_url }}</a>
            </template>
          </el-table-column>
          <el-table-column label="目标地址" min-width="200">
            <template #default="{ row }">
              <div v-if="row.title" class="title-cell">
                <span class="title">{{ row.title }}</span>
                <span class="long">{{ row.long_url }}</span>
              </div>
              <span v-else class="long">{{ row.long_url }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="clicks" label="点击" width="80" align="center" />
          <el-table-column label="有效期" width="120">
            <template #default="{ row }">
              <el-tag v-if="isExpired(row.expire_at)" size="small" type="danger" effect="plain">已过期</el-tag>
              <el-tag
                v-else-if="row.expire_at"
                size="small"
                :type="expiryDaysLeft(row.expire_at) <= 7 ? 'danger' : 'warning'"
                effect="plain"
              >
                {{ expiryLabel(row.expire_at) }}
              </el-tag>
              <span v-else class="mono">永久</span>
            </template>
          </el-table-column>
          <el-table-column label="创建时间" width="150">
            <template #default="{ row }">
              <span class="mono">{{ dayjs(row.created_at).format('YYYY-MM-DD HH:mm') }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="160" align="center">
            <template #default="{ row }">
              <div class="ops">
                <el-button :icon="CopyDocument" circle size="small" title="复制短链" @click="copy(row.short_url)" />
                <el-button :icon="Delete" circle size="small" type="danger" plain title="删除" @click="handleDelete(row as MemberLink)" />
                <el-dropdown trigger="click" @command="(cmd: string) => handleMore(cmd, row as MemberLink)">
                  <el-button size="small" title="更多">
                    更多<el-icon class="el-icon--right"><ArrowDown /></el-icon>
                  </el-button>
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item command="edit">编辑</el-dropdown-item>
                      <el-dropdown-item command="expiry">有效期</el-dropdown-item>
                      <el-dropdown-item command="qr">二维码</el-dropdown-item>
                      <el-dropdown-item command="stats">统计</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <el-empty description="还没有短链，先在上方创建一个" />
          </template>
        </el-table>
        <div class="pager">
          <el-pagination
            v-model:current-page="page"
            :total="total"
            :page-size="20"
            layout="prev, pager, next"
            background
            @current-change="load"
          />
        </div>
      </section>
    </main>

    <el-dialog v-model="statsVisible" title="短链统计" width="560px" :close-on-click-modal="false">
      <div v-loading="statsLoading" class="stats-body">
        <template v-if="stats">
          <div class="stats-total">
            <span class="stats-total-num">{{ stats.total.toLocaleString() }}</span>
            <span class="stats-total-label">总点击</span>
          </div>

          <div class="stats-block">
            <div class="stats-block-title">近 7 天趋势</div>
            <div v-if="stats.trend.length" class="trend-bars">
              <div v-for="t in stats.trend" :key="t.date" class="trend-col">
                <div class="trend-bar" :style="{ height: (t.clicks / maxTrend()) * 100 + '%' }">
                  <span class="trend-val">{{ t.clicks }}</span>
                </div>
                <span class="trend-date">{{ t.date.slice(5) }}</span>
              </div>
            </div>
            <p v-else class="stats-empty">近 7 天暂无点击</p>
          </div>

          <div class="stats-block">
            <div class="stats-block-title">来源 Top 10</div>
            <el-table v-if="stats.referrers.length" :data="stats.referrers" size="small">
              <el-table-column label="来源" min-width="200">
                <template #default="{ row }">
                  <span class="mono">{{ row.referrer || '直接访问' }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="count" label="次数" width="90" align="center" />
            </el-table>
            <p v-else class="stats-empty">暂无来源数据</p>
          </div>

          <div class="stats-block">
            <div class="stats-block-title">来源类型</div>
            <el-table v-if="stats.referrer_types && stats.referrer_types.length" :data="stats.referrer_types" size="small">
              <el-table-column label="类型" min-width="120">
                <template #default="{ row }">
                  <span>{{ refTypeIcon(row.device) }}</span>
                  <span style="margin-left: 6px">{{ row.device }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="count" label="次数" width="90" align="center" />
            </el-table>
            <p v-else class="stats-empty">暂无来源类型数据</p>
          </div>

          <div class="stats-block">
            <div class="stats-block-title">设备分布</div>
            <el-table v-if="stats.devices.length" :data="stats.devices" size="small">
              <el-table-column label="设备" min-width="120">
                <template #default="{ row }">
                  <el-icon :size="14" style="vertical-align: -2px; margin-right: 4px">
                    {{ row.device === '手机' ? '📱' : row.device === '平板' ? '📲' : '💻' }}
                  </el-icon>
                  <span>{{ row.device }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="count" label="次数" width="90" align="center" />
            </el-table>
            <p v-else class="stats-empty">暂无设备数据</p>
          </div>

          <div class="stats-block">
            <div class="stats-block-title">浏览器分布</div>
            <el-table v-if="stats.browsers.length" :data="stats.browsers" size="small">
              <el-table-column label="浏览器" min-width="120">
                <template #default="{ row }">
                  <span>{{ row.device }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="count" label="次数" width="90" align="center" />
            </el-table>
            <p v-else class="stats-empty">暂无浏览器数据</p>
          </div>

          <div class="stats-block">
            <div class="stats-block-title">地域分布</div>
            <el-table v-if="stats.countries.length" :data="stats.countries" size="small">
              <el-table-column label="国家/地区" min-width="120">
                <template #default="{ row }">
                  <span>{{ countryFlag(row.device) }}</span>
                  <span style="margin-left: 6px">{{ countryName(row.device) }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="count" label="次数" width="90" align="center" />
            </el-table>
            <p v-else class="stats-empty">暂无地域数据</p>
          </div>
        </template>
      </div>
    </el-dialog>

    <el-dialog v-model="createdVisible" title="短链创建成功" width="440px" align-center :close-on-click-modal="false">
      <div class="create-result">
        <a :href="createdResult" target="_blank" rel="noopener" class="result-link mono">{{ createdResult }}</a>
      </div>
      <template #footer>
        <el-button @click="createdVisible = false">关闭</el-button>
        <el-button @click="copy(createdResult)">复制</el-button>
        <el-button type="primary" @click="openQr(createdResult)">二维码</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="qrVisible" title="二维码" width="320px" align-center>
      <div class="qr-box">
        <div ref="qrBox" class="qr-canvas"></div>
        <p class="qr-hint mono">{{ qrUrl }}</p>
      </div>
      <template #footer>
        <el-button @click="qrVisible = false">关闭</el-button>
        <el-button type="primary" @click="downloadQr">下载图片</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="expiryVisible" title="修改有效期" width="360px" align-center>
      <el-form label-position="top">
        <el-form-item label="短链">
          <span class="mono">{{ expiryLink?.short_url }}</span>
        </el-form-item>
        <el-form-item label="有效期">
          <el-select v-model="expiryDays" style="width: 100%">
            <el-option label="永久有效" :value="0" />
            <el-option label="1 天" :value="1" />
            <el-option label="7 天" :value="7" />
            <el-option label="30 天" :value="30" />
            <el-option label="1 年" :value="365" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="expiryVisible = false">取消</el-button>
        <el-button type="primary" :loading="expirySaving" @click="saveExpiry">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchVisible" title="批量创建短链" width="560px" :close-on-click-modal="false">
      <el-input
        v-model="batchText"
        type="textarea"
        :rows="8"
        placeholder="每行一个网址，最多 100 条"
      />
      <div v-if="batchResults.length" class="batch-results">
        <div v-for="(r, i) in batchResults" :key="i" class="batch-row">
          <el-tag v-if="r.error" type="danger" size="small" effect="plain">失败</el-tag>
          <el-tag v-else type="success" size="small" effect="plain">成功</el-tag>
          <span class="mono batch-url">{{ r.url }}</span>
          <a v-if="r.short_url" :href="r.short_url" target="_blank" rel="noopener" class="mono batch-out">{{ r.short_url }}</a>
          <span v-else class="batch-err">{{ r.error }}</span>
        </div>
      </div>
      <template #footer>
        <el-button @click="batchVisible = false">关闭</el-button>
        <el-button type="primary" :loading="batchLoading" @click="submitBatch">批量创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importVisible" title="导入短链 (CSV)" width="560px" :close-on-click-modal="false">
      <p class="field-meta">每行一条：<code class="mono">url,标题,自定义短码,有效期天数</code></p>
      <el-input
        v-model="importText"
        type="textarea"
        :rows="8"
        placeholder="https://example.com/a,标题A,,7&#10;https://example.com/b,,,0"
      />
      <div v-if="importResults.length" class="batch-results">
        <div v-for="(r, i) in importResults" :key="i" class="batch-row">
          <el-tag v-if="r.error" type="danger" size="small" effect="plain">失败</el-tag>
          <el-tag v-else type="success" size="small" effect="plain">成功</el-tag>
          <span class="mono batch-url">{{ r.url }}</span>
          <a v-if="r.short_url" :href="r.short_url" target="_blank" rel="noopener" class="mono batch-out">{{ r.short_url }}</a>
          <span v-else class="batch-err">{{ r.error }}</span>
        </div>
      </div>
      <template #footer>
        <el-button @click="importVisible = false">关闭</el-button>
        <el-button type="primary" :loading="importing" @click="submitImport">开始导入</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editVisible" title="编辑短链" width="420px" align-center :close-on-click-modal="false">
      <el-form label-position="top">
        <el-form-item label="短链">
          <span class="mono">{{ editLink?.short_url }}</span>
        </el-form-item>
        <el-form-item label="目标地址">
          <el-input v-model="editForm.long_url" placeholder="粘贴新的目标网址" />
        </el-form-item>
        <el-form-item label="标题（可选）">
          <el-input v-model="editForm.title" placeholder="给这条短链起个名字" maxlength="64" />
        </el-form-item>
        <el-form-item label="有效期">
          <el-select v-model="editForm.expire_days" style="width: 100%">
            <el-option label="永久有效" :value="0" />
            <el-option label="1 天" :value="1" />
            <el-option label="7 天" :value="7" />
            <el-option label="30 天" :value="30" />
            <el-option label="1 年" :value="365" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="editSaving" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.shell {
  max-width: 1080px;
  margin: 0 auto;
  padding: 20px;
}

.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 0;
  border-bottom: 1px solid #e4e7ea;
  margin-bottom: 20px;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 700;
  color: var(--dwz-petrol);
}

.brand-mark {
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  background: var(--dwz-petrol);
  color: #fff;
  border-radius: 8px;
}

.user {
  display: flex;
  align-items: center;
  gap: 10px;
}

.user-name {
  font-weight: 600;
}

.home-link {
  font-size: 13px;
  color: var(--dwz-petrol);
  text-decoration: none;
}

.home-link:hover {
  text-decoration: underline;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin-bottom: 16px;
}

.verify-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  margin-bottom: 16px;
  background: #fff7e6;
  border: 1px solid #ffd591;
  border-radius: 10px;
  color: #874d00;
  font-size: 13px;
  flex-wrap: wrap;
}

.summary-card {
  background: #fff;
  border: 1px solid #e4e7ea;
  border-radius: 12px;
  padding: 18px;
  text-align: center;
}

.summary-num {
  display: block;
  font-size: 28px;
  font-weight: 700;
  color: var(--dwz-petrol);
  font-variant-numeric: tabular-nums;
}

.summary-label {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  color: var(--dwz-text-dim);
}

.card {
  background: #fff;
  border: 1px solid #e4e7ea;
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 16px;
}

.card h2 {
  margin: 0 0 16px;
  font-size: 16px;
  color: #111;
}

.create-row {
  display: flex;
  gap: 10px;
}

.ops {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.url-input {
  flex: 1;
}

.title-input {
  width: 150px;
}

.custom-input {
  width: 170px;
}

.expire-select {
  width: 130px;
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.card-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.search-input {
  width: 200px;
}

.status-select {
  width: 130px;
}

.short-link {
  color: var(--dwz-petrol);
  font-weight: 600;
  text-decoration: none;
}

.title-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.title {
  font-weight: 600;
  color: var(--dwz-text);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 260px;
}

.long {
  color: var(--dwz-text-dim);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 260px;
  display: inline-block;
}

.mono {
  font-family: monospace;
  font-size: 12px;
  color: #666;
}

.pager {
  margin-top: 14px;
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 720px) {
  .create-row {
    flex-direction: column;
  }
  .custom-input,
  .expire-select {
    width: 100%;
  }
  .summary-grid {
    grid-template-columns: 1fr;
  }
}

.stats-body {
  min-height: 120px;
}

.stats-total {
  text-align: center;
  padding: 8px 0 18px;
}

.stats-total-num {
  font-size: 40px;
  font-weight: 700;
  color: var(--dwz-petrol);
  font-variant-numeric: tabular-nums;
}

.stats-total-label {
  display: block;
  margin-top: 2px;
  color: var(--dwz-text-dim);
  font-size: 12px;
}

.stats-block {
  margin-top: 18px;
}

.stats-block-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--dwz-text-dim);
  margin-bottom: 10px;
}

.trend-bars {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  height: 140px;
  padding: 0 4px;
}

.trend-col {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
  height: 100%;
}

.trend-bar {
  width: 100%;
  max-width: 34px;
  min-height: 2px;
  background: linear-gradient(180deg, #2aa3ab, #0e6e75);
  border-radius: 4px 4px 0 0;
  position: relative;
}

.trend-val {
  position: absolute;
  top: -18px;
  left: 50%;
  transform: translateX(-50%);
  font-size: 11px;
  color: #666;
}

.trend-date {
  margin-top: 6px;
  font-size: 11px;
  color: #999;
}

.stats-empty {
  color: #999;
  font-size: 13px;
  padding: 12px 0;
}

.create-result {
  text-align: center;
  padding: 14px;
}

.create-result .result-link {
  display: inline-block;
  padding: 10px 14px;
  background: #f4f8f9;
  border: 1px solid var(--dwz-line, #e4e7ea);
  border-radius: 8px;
  color: var(--dwz-petrol);
  font-weight: 600;
  text-decoration: none;
  word-break: break-all;
}

.qr-box {
  text-align: center;
}

.qr-canvas {
  display: inline-block;
  padding: 10px;
  background: #fff;
  border: 1px solid var(--dwz-line, #e4e7ea);
  border-radius: 10px;
}

.qr-hint {
  margin-top: 12px;
  font-size: 12px;
  color: var(--dwz-text-dim);
  word-break: break-all;
}

.batch-results {
  margin-top: 14px;
  max-height: 260px;
  overflow-y: auto;
  border: 1px solid var(--dwz-line, #e4e7ea);
  border-radius: 8px;
  padding: 8px;
}

.batch-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 4px;
  border-bottom: 1px solid #f0f2f3;
}

.batch-row:last-child {
  border-bottom: none;
}

.batch-url {
  flex: 1;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.batch-out {
  font-size: 12px;
  color: var(--dwz-petrol);
  text-decoration: none;
}

.batch-err {
  font-size: 12px;
  color: #dc2626;
}
</style>