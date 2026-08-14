<script setup lang="ts">
// 单链统计弹窗（admin 短链管理 + 会员中心共用）。
// 接收已归一化的统计对象（{label, clicks} 结构），纯展示组件。

export interface NormalizedStat {
  total: number
  trend: { label: string; clicks: number }[]
  referrers: { label: string; clicks: number }[]
  referrer_types: { label: string; clicks: number }[]
  devices: { label: string; clicks: number }[]
  browsers: { label: string; clicks: number }[]
  countries: { label: string; clicks: number }[]
}

const props = defineProps<{
  modelValue: boolean
  loading: boolean
  stats: NormalizedStat | null
}>()

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v)
})

function statsMax(items: { clicks: number }[]): number {
  return items.reduce((m, t) => Math.max(m, t.clicks), 0) || 1
}

/** ISO 3166-1 alpha-2 → 中文名（地域分布展示） */
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
</script>

<template>
  <el-dialog v-model="visible" title="短链统计" width="560px" :close-on-click-modal="false">
    <div v-loading="loading" class="stats-body">
      <template v-if="stats">
        <div class="stats-total">
          <span class="stats-total-num">{{ stats.total.toLocaleString() }}</span>
          <span class="stats-total-label">总点击</span>
        </div>

        <div class="stats-block">
          <div class="stats-block-title">近 7 天趋势</div>
          <div v-if="stats.trend.length" class="trend-bars">
            <div v-for="t in stats.trend" :key="t.label" class="trend-col">
              <div class="trend-bar" :style="{ height: (t.clicks / statsMax(stats.trend)) * 100 + '%' }">
                <span class="trend-val">{{ t.clicks }}</span>
              </div>
              <span class="trend-date">{{ t.label.slice(5) }}</span>
            </div>
          </div>
          <p v-else class="stats-empty">近 7 天暂无点击</p>
        </div>

        <div class="stats-block">
          <div class="stats-block-title">来源 Top 10</div>
          <el-table v-if="stats.referrers.length" :data="stats.referrers" size="small">
            <el-table-column label="来源" min-width="200">
              <template #default="{ row }">
                <span class="mono">{{ row.label || '直接访问' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="clicks" label="次数" width="90" align="center" />
          </el-table>
          <p v-else class="stats-empty">暂无来源数据</p>
        </div>

        <div class="stats-block">
          <div class="stats-block-title">来源类型</div>
          <el-table v-if="stats.referrer_types.length" :data="stats.referrer_types" size="small">
            <el-table-column label="类型" min-width="120">
              <template #default="{ row }">
                <span>{{ refTypeIcon(row.label) }}</span>
                <span style="margin-left: 6px">{{ row.label }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="clicks" label="次数" width="90" align="center" />
          </el-table>
          <p v-else class="stats-empty">暂无来源类型数据</p>
        </div>

        <div class="stats-block">
          <div class="stats-block-title">设备分布</div>
          <el-table v-if="stats.devices.length" :data="stats.devices" size="small">
            <el-table-column label="设备" min-width="120">
              <template #default="{ row }">
                <span style="margin-right: 4px">{{ row.label === '手机' ? '📱' : row.label === '平板' ? '📲' : '💻' }}</span>
                <span>{{ row.label }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="clicks" label="次数" width="90" align="center" />
          </el-table>
          <p v-else class="stats-empty">暂无设备数据</p>
        </div>

        <div class="stats-block">
          <div class="stats-block-title">浏览器分布</div>
          <el-table v-if="stats.browsers.length" :data="stats.browsers" size="small">
            <el-table-column label="浏览器" min-width="120">
              <template #default="{ row }">
                <span>{{ row.label }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="clicks" label="次数" width="90" align="center" />
          </el-table>
          <p v-else class="stats-empty">暂无浏览器数据</p>
        </div>

        <div class="stats-block">
          <div class="stats-block-title">地域分布</div>
          <el-table v-if="stats.countries.length" :data="stats.countries" size="small">
            <el-table-column label="国家/地区" min-width="120">
              <template #default="{ row }">
                <span>{{ countryFlag(row.label) }}</span>
                <span style="margin-left: 6px">{{ countryName(row.label) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="clicks" label="次数" width="90" align="center" />
          </el-table>
          <p v-else class="stats-empty">暂无地域数据</p>
        </div>
      </template>
    </div>
  </el-dialog>
</template>

<style scoped>
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
  color: var(--dwz-text-dim);
}

.trend-date {
  margin-top: 6px;
  font-size: 11px;
  color: var(--dwz-text-dim);
}

.stats-empty {
  color: var(--dwz-text-dim);
  font-size: 13px;
  padding: 12px 0;
}
</style>
