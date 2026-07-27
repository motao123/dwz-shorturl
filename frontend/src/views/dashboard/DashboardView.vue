<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Link, Sunny, Pointer, PieChart, Refresh } from '@element-plus/icons-vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, DataZoomComponent } from 'echarts/components'
import type { ComposeOption } from 'echarts/core'
import type { LineSeriesOption } from 'echarts/charts'
import type { GridComponentOption, TooltipComponentOption, DataZoomComponentOption } from 'echarts/components'
import { getOverview, getTrend } from '@/api/stats'
import type { StatsOverview } from '@/api/stats'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, DataZoomComponent])
type ECOption = ComposeOption<LineSeriesOption | GridComponentOption | TooltipComponentOption | DataZoomComponentOption>

const loading = ref(false)
const chartLoading = ref(false)

const overview = reactive<StatsOverview>({
  total_urls: 0,
  total_clicks: 0,
  today_new: 0,
  today_clicks: 0,
  active_rate: 0
})

/** 数字滚动动画 */
const display = reactive({ total: 0, newToday: 0, clicksToday: 0, rate: 0 })
const animTimers: number[] = []

function animateTo(key: keyof typeof display, target: number, decimals = 0, duration = 900) {
  const start = display[key]
  const startTime = performance.now()
  const step = (now: number) => {
    const t = Math.min((now - startTime) / duration, 1)
    const eased = 1 - Math.pow(1 - t, 3)
    const value = start + (target - start) * eased
    display[key] = decimals > 0 ? Number(value.toFixed(decimals)) : Math.round(value)
    if (t < 1) animTimers.push(requestAnimationFrame(step))
  }
  animTimers.push(requestAnimationFrame(step))
}

function rateToPercent(rate: number): number {
  return rate > 1 ? rate : Number((rate * 100).toFixed(1))
}

async function loadOverview() {
  loading.value = true
  try {
    const data = await getOverview()
    if (data) {
      Object.assign(overview, data)
      animateTo('total', data.total_urls ?? 0)
      animateTo('newToday', data.today_new ?? 0)
      animateTo('clicksToday', data.today_clicks ?? 0)
      animateTo('rate', rateToPercent(data.active_rate ?? 0), 1)
    }
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '概览数据加载失败')
  } finally {
    loading.value = false
  }
}

const chartOption = ref<ECOption>({})

async function loadTrend() {
  chartLoading.value = true
  try {
    const points = (await getTrend({ granularity: 'day', days: 7 })) ?? []
    chartOption.value = {
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(12, 42, 48, 0.92)',
        borderWidth: 0,
        textStyle: { color: '#e8f4f2', fontSize: 12.5 },
        axisPointer: { type: 'line', lineStyle: { color: '#f5a623', width: 1, type: 'dashed' } }
      },
      grid: { left: 8, right: 18, top: 26, bottom: 6, containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: points.map((p) => p.label),
        axisLine: { lineStyle: { color: '#dfe7ea' } },
        axisTick: { show: false },
        axisLabel: { color: '#6b7f86', fontSize: 11.5, fontFamily: 'JetBrains Mono' }
      },
      yAxis: {
        type: 'value',
        minInterval: 1,
        axisLabel: { color: '#6b7f86', fontSize: 11.5, fontFamily: 'JetBrains Mono' },
        splitLine: { lineStyle: { color: '#e8eef0', type: 'dashed' } }
      },
      series: [
        {
          name: '点击量',
          type: 'line',
          smooth: true,
          symbol: 'circle',
          symbolSize: 7,
          data: points.map((p) => p.clicks),
          lineStyle: { width: 3, color: '#0e6e75' },
          itemStyle: { color: '#0e6e75', borderColor: '#fff', borderWidth: 2 },
          emphasis: { itemStyle: { color: '#f5a623', borderColor: '#fff' } },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0, y: 0, x2: 0, y2: 1,
              colorStops: [
                { offset: 0, color: 'rgba(14, 110, 117, 0.22)' },
                { offset: 1, color: 'rgba(14, 110, 117, 0.01)' }
              ]
            }
          }
        }
      ]
    }
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '趋势数据加载失败')
  } finally {
    chartLoading.value = false
  }
}

function reload() {
  loadOverview()
  loadTrend()
}

interface StatCard {
  key: keyof typeof display
  label: string
  sub: string
  icon: typeof Link
  tone: string
  suffix?: string
}

const statCards: StatCard[] = [
  { key: 'total', label: '短链总数', sub: 'SHORT URLS', icon: Link, tone: 'petrol' },
  { key: 'newToday', label: '今日新增', sub: 'NEW TODAY', icon: Sunny, tone: 'amber' },
  { key: 'clicksToday', label: '今日点击', sub: 'CLICKS TODAY', icon: Pointer, tone: 'green' },
  { key: 'rate', label: '活跃率', sub: 'ACTIVE RATE', icon: PieChart, tone: 'steel', suffix: '%' }
]

onMounted(reload)
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          仪表盘
          <small>OVERVIEW · 运营全景</small>
        </h1>
      </div>
      <el-button :icon="Refresh" circle @click="reload" title="刷新数据" />
    </div>

    <!-- 指标卡 -->
    <div class="stats-grid">
      <div v-for="card in statCards" :key="card.key" class="stat" :class="`stat--${card.tone}`">
        <div class="stat__icon">
          <el-icon :size="22"><component :is="card.icon" /></el-icon>
        </div>
        <div class="stat__body">
          <span class="stat__label mono">{{ card.sub }}</span>
          <span class="stat__value mono">
            <template v-if="loading">··</template>
            <template v-else>
              {{ display[card.key].toLocaleString() }}<i v-if="card.suffix">{{ card.suffix }}</i>
            </template>
          </span>
          <span class="stat__name">{{ card.label }}</span>
        </div>
      </div>
    </div>

    <!-- 趋势图 -->
    <section class="chart-card app-card">
      <header class="chart-card__head">
        <div>
          <h3 class="chart-card__title">近 7 日点击趋势</h3>
          <p class="chart-card__sub mono">CLICK TREND · DAILY</p>
        </div>
        <span class="chart-card__legend mono">
          <i></i>点击量
        </span>
      </header>
      <div v-loading="chartLoading" class="chart-card__body">
        <VChart v-if="Object.keys(chartOption).length" :option="chartOption" autoresize class="chart" />
        <el-empty v-else-if="!chartLoading" description="暂无趋势数据" />
      </div>
    </section>
  </div>
</template>

<style scoped>
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
  gap: 16px;
  margin-bottom: 18px;
}

.stat {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 22px;
  background: #fff;
  border: 1px solid var(--dwz-line);
  border-radius: 14px;
  overflow: hidden;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.stat::before {
  content: '';
  position: absolute;
  inset: 0 auto 0 0;
  width: 4px;
  background: var(--tone);
}

.stat:hover {
  transform: translateY(-3px);
  box-shadow: 0 12px 28px rgba(12, 42, 48, 0.09);
}

.stat--petrol { --tone: #0e6e75; }
.stat--amber { --tone: #f5a623; }
.stat--green { --tone: #16a34a; }
.stat--steel { --tone: #3d6b9e; }

.stat__icon {
  width: 48px;
  height: 48px;
  flex-shrink: 0;
  display: grid;
  place-items: center;
  border-radius: 13px;
  color: var(--tone);
  background: color-mix(in srgb, var(--tone) 11%, white);
  border: 1px solid color-mix(in srgb, var(--tone) 22%, white);
}

.stat__body {
  display: flex;
  flex-direction: column;
  line-height: 1.3;
  min-width: 0;
}

.stat__label {
  font-size: 9.5px;
  letter-spacing: 0.18em;
  color: var(--dwz-text-dim);
}

.stat__value {
  font-size: 30px;
  font-weight: 700;
  letter-spacing: -0.03em;
  color: var(--dwz-ink);
  font-variant-numeric: tabular-nums;
}

.stat__value i {
  font-style: normal;
  font-size: 16px;
  color: var(--dwz-text-dim);
  margin-left: 1px;
}

.stat__name {
  font-size: 12.5px;
  color: var(--dwz-text-dim);
}

/* ---------------- 图表卡 ---------------- */

.chart-card {
  padding: 20px 22px 14px;
}

.chart-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 8px;
}

.chart-card__title {
  margin: 0;
  font-size: 16.5px;
  font-weight: 800;
  color: var(--dwz-ink);
}

.chart-card__sub {
  margin: 3px 0 0;
  font-size: 10px;
  letter-spacing: 0.2em;
  color: var(--dwz-text-dim);
}

.chart-card__legend {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 11.5px;
  color: var(--dwz-text-dim);
}

.chart-card__legend i {
  width: 16px;
  height: 4px;
  border-radius: 2px;
  background: linear-gradient(90deg, #0e6e75, #f5a623);
}

.chart-card__body {
  height: 340px;
}

.chart {
  width: 100%;
  height: 100%;
}
</style>
