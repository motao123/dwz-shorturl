<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, DocumentCopy } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import type { ComposeOption } from 'echarts/core'
import type { LineSeriesOption, BarSeriesOption } from 'echarts/charts'
import type { GridComponentOption, TooltipComponentOption } from 'echarts/components'
import { getTrend, getTop, getRecent, type TrendGranularity, type RecentUrl } from '@/api/stats'
import { buildShortUrl, SOURCE_LABELS } from '@/utils/constants'
import { copyText } from '@/utils/clipboard'

use([CanvasRenderer, LineChart, BarChart, GridComponent, TooltipComponent])
type ECOption = ComposeOption<LineSeriesOption | BarSeriesOption | GridComponentOption | TooltipComponentOption>

const loading = ref(false)
const topLoading = ref(false)
const recentLoading = ref(false)

const granularity = ref<TrendGranularity>('day')
const dateRange = ref<[string, string]>([
  dayjs().subtract(13, 'day').format('YYYY-MM-DD'),
  dayjs().format('YYYY-MM-DD')
])

const trendOption = ref<ECOption>({})
const topOption = ref<ECOption>({})
const recentRows = ref<RecentUrl[]>([])

function dateParams() {
  return {
    date_from: dateRange.value?.[0] ? dayjs(dateRange.value[0]).format('YYYY-MM-DD') : undefined,
    date_to: dateRange.value?.[1] ? dayjs(dateRange.value[1]).format('YYYY-MM-DD') : undefined
  }
}

async function loadTrend() {
  loading.value = true
  try {
    const points = (await getTrend({ granularity: granularity.value, ...dateParams() })) ?? []
    trendOption.value = {
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(12, 42, 48, 0.92)',
        borderWidth: 0,
        textStyle: { color: '#e8f4f2', fontSize: 12.5 },
        axisPointer: { type: 'line', lineStyle: { color: '#f5a623', type: 'dashed' } }
      },
      grid: { left: 8, right: 18, top: 24, bottom: 8, containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: points.map((p) => p.label),
        axisLine: { lineStyle: { color: '#dfe7ea' } },
        axisTick: { show: false },
        axisLabel: { color: '#6b7f86', fontSize: 11, fontFamily: 'JetBrains Mono' }
      },
      yAxis: {
        type: 'value',
        minInterval: 1,
        axisLabel: { color: '#6b7f86', fontSize: 11, fontFamily: 'JetBrains Mono' },
        splitLine: { lineStyle: { color: '#e8eef0', type: 'dashed' } }
      },
      series: [
        {
          name: '点击量',
          type: 'line',
          smooth: true,
          symbol: 'circle',
          symbolSize: 6,
          data: points.map((p) => p.clicks),
          lineStyle: { width: 3, color: '#0e6e75' },
          itemStyle: { color: '#0e6e75', borderColor: '#fff', borderWidth: 2 },
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
        },
        {
          name: '新增短链',
          type: 'line',
          smooth: true,
          symbol: 'circle',
          symbolSize: 5,
          data: points.map((p) => p.new_urls ?? 0),
          lineStyle: { width: 2, color: '#f5a623', type: 'dashed' },
          itemStyle: { color: '#f5a623', borderColor: '#fff', borderWidth: 1.5 }
        }
      ]
    }
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : '趋势数据加载失败')
  } finally {
    loading.value = false
  }
}

async function loadTop() {
  topLoading.value = true
  try {
    const tops = (await getTop({ limit: 10, ...dateParams() })) ?? []
    const items = [...tops].reverse()
    topOption.value = {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        backgroundColor: 'rgba(12, 42, 48, 0.92)',
        borderWidth: 0,
        textStyle: { color: '#e8f4f2', fontSize: 12.5 },
        formatter: (params: unknown) => {
          const p = (params as { dataIndex: number; value: number }[])[0]
          const item = items[p.dataIndex]
          return `<b class="mono">${item.uid}</b><br/>点击 ${p.value.toLocaleString()} 次`
        }
      },
      grid: { left: 8, right: 40, top: 8, bottom: 8, containLabel: true },
      xAxis: {
        type: 'value',
        minInterval: 1,
        axisLabel: { color: '#6b7f86', fontSize: 11, fontFamily: 'JetBrains Mono' },
        splitLine: { lineStyle: { color: '#e8eef0', type: 'dashed' } }
      },
      yAxis: {
        type: 'category',
        data: items.map((t) => t.title || t.uid),
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: {
          color: '#1f3238',
          fontSize: 11.5,
          fontFamily: 'JetBrains Mono',
          width: 110,
          overflow: 'truncate'
        }
      },
      series: [
        {
          name: '点击量',
          type: 'bar',
          barWidth: 14,
          data: items.map((t) => t.clicks),
          itemStyle: {
            borderRadius: [0, 6, 6, 0],
            color: {
              type: 'linear',
              x: 0, y: 0, x2: 1, y2: 0,
              colorStops: [
                { offset: 0, color: '#0e6e75' },
                { offset: 1, color: '#2fa3a8' }
              ]
            }
          },
          label: {
            show: true,
            position: 'right',
            color: '#6b7f86',
            fontSize: 11,
            fontFamily: 'JetBrains Mono',
            formatter: (p: { value?: unknown }) => Number(p.value ?? 0).toLocaleString()
          }
        }
      ]
    }
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Top 榜单加载失败')
  } finally {
    topLoading.value = false
  }
}

async function loadRecent() {
  recentLoading.value = true
  try {
    const res = await getRecent(20)
    recentRows.value = Array.isArray(res) ? res : ((res as unknown as { list: RecentUrl[] })?.list ?? [])
  } catch (err) {
    recentRows.value = []
    ElMessage.error(err instanceof Error ? err.message : '最近创建加载失败')
  } finally {
    recentLoading.value = false
  }
}

function reload() {
  loadTrend()
  loadTop()
  loadRecent()
}

function handleFilterChange() {
  loadTrend()
  loadTop()
}

async function handleCopy(uid: string) {
  try {
    await copyText(buildShortUrl(uid))
    ElMessage.success('已复制短链')
  } catch {
    ElMessage.error('复制失败')
  }
}

onMounted(reload)
</script>

<template>
  <div class="app-page">
    <div class="app-page__head">
      <div>
        <h1 class="app-page__title">
          统计分析
          <small>ANALYTICS · 趋势 / Top 榜 / 最近创建</small>
        </h1>
      </div>
      <div class="filter-bar">
        <el-radio-group v-model="granularity" size="default" @change="handleFilterChange">
          <el-radio-button value="hour">按小时</el-radio-button>
          <el-radio-button value="day">按天</el-radio-button>
          <el-radio-button value="month">按月</el-radio-button>
        </el-radio-group>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          range-separator="至"
          start-placeholder="开始"
          end-placeholder="结束"
          value-format="YYYY-MM-DD"
          :shortcuts="[
            { text: '近 7 天', value: () => [dayjs().subtract(6, 'day').toDate(), new Date()] },
            { text: '近 30 天', value: () => [dayjs().subtract(29, 'day').toDate(), new Date()] },
            { text: '近 90 天', value: () => [dayjs().subtract(89, 'day').toDate(), new Date()] }
          ]"
          style="width: 260px"
          @change="handleFilterChange"
        />
        <el-button :icon="Refresh" circle title="刷新" @click="reload" />
      </div>
    </div>

    <!-- 趋势图 -->
    <section class="app-card chart-card">
      <header class="chart-card__head">
        <h3 class="chart-card__title">点击趋势</h3>
        <span class="chart-card__legend mono"><i class="lg lg--clicks"></i>点击量<i class="lg lg--new"></i>新增短链</span>
      </header>
      <div v-loading="loading" class="chart-box">
        <VChart v-if="Object.keys(trendOption).length" :option="trendOption" autoresize class="chart" />
        <el-empty v-else-if="!loading" description="暂无数据" />
      </div>
    </section>

    <div class="split">
      <!-- Top 10 -->
      <section class="app-card chart-card">
        <header class="chart-card__head">
          <h3 class="chart-card__title">热门 Top 10</h3>
          <span class="chart-card__sub mono">BY CLICKS</span>
        </header>
        <div v-loading="topLoading" class="chart-box chart-box--bar">
          <VChart v-if="Object.keys(topOption).length" :option="topOption" autoresize class="chart" />
          <el-empty v-else-if="!topLoading" description="暂无数据" />
        </div>
      </section>

      <!-- 最近创建 -->
      <section class="app-card chart-card">
        <header class="chart-card__head">
          <h3 class="chart-card__title">最近创建</h3>
          <span class="chart-card__sub mono">RECENT 20</span>
        </header>
        <div v-loading="recentLoading" class="recent-wrap">
          <table class="recent">
            <tbody>
              <tr v-for="row in recentRows" :key="row.id">
                <td>
                  <a :href="buildShortUrl(row.uid)" target="_blank" rel="noopener" class="mono recent__uid">
                    {{ row.uid }}
                  </a>
                </td>
                <td class="recent__src">
                  <el-tag size="small" effect="plain" round>{{ SOURCE_LABELS[row.source] ?? row.source }}</el-tag>
                </td>
                <td class="mono recent__clicks">{{ row.clicks.toLocaleString() }}</td>
                <td class="mono recent__time">{{ dayjs(row.created_at).format('MM-DD HH:mm') }}</td>
                <td>
                  <button class="mini-btn" title="复制" @click="handleCopy(row.uid)">
                    <el-icon :size="12"><DocumentCopy /></el-icon>
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
          <el-empty v-if="!recentLoading && !recentRows.length" description="暂无数据" />
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.chart-card {
  padding: 18px 20px 12px;
  margin-bottom: 16px;
}

.chart-card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.chart-card__title {
  margin: 0;
  font-size: 15.5px;
  font-weight: 800;
  color: var(--dwz-ink);
}

.chart-card__sub {
  font-size: 10px;
  letter-spacing: 0.2em;
  color: var(--dwz-text-dim);
}

.chart-card__legend {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  color: var(--dwz-text-dim);
}

.lg {
  display: inline-block;
  width: 14px;
  height: 4px;
  border-radius: 2px;
  margin-left: 8px;
}

.lg--clicks { background: #0e6e75; margin-left: 0; }
.lg--new { background: #f5a623; }

.chart-box {
  height: 320px;
}

.chart-box--bar {
  height: 380px;
}

.chart {
  width: 100%;
  height: 100%;
}

.split {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  align-items: start;
}

@media (max-width: 1100px) {
  .split {
    grid-template-columns: 1fr;
  }
}

/* 最近创建紧凑表格 */
.recent-wrap {
  max-height: 392px;
  overflow-y: auto;
}

.recent {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
}

.recent td {
  padding: 8px 6px;
  border-bottom: 1px solid #eef3f4;
  vertical-align: middle;
}

.recent tr:last-child td {
  border-bottom: none;
}

.recent tr {
  transition: background-color 0.14s ease;
}

.recent tr:hover {
  background: #f4f9f9;
}

.recent__uid {
  color: var(--dwz-petrol);
  font-weight: 700;
  text-decoration: none;
}

.recent__uid:hover {
  color: var(--dwz-amber-deep);
}

.recent__clicks {
  text-align: right;
  font-weight: 700;
  color: var(--dwz-ink);
  white-space: nowrap;
}

.recent__time {
  color: var(--dwz-text-dim);
  white-space: nowrap;
  text-align: right;
}

.recent__src {
  text-align: center;
}

.mini-btn {
  display: inline-grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border: 1px solid var(--dwz-line);
  border-radius: 6px;
  background: #fff;
  color: var(--dwz-text-dim);
  cursor: pointer;
  transition: all 0.15s ease;
}

.mini-btn:hover {
  color: var(--dwz-petrol);
  border-color: var(--dwz-petrol);
}
</style>
