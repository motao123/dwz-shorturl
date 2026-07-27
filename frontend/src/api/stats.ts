import request from './request'

export interface StatsOverview {
  total_urls: number
  total_clicks: number
  today_new: number
  today_clicks: number
  active_rate: number
}

export type TrendGranularity = 'hour' | 'day' | 'month'

export interface TrendPoint {
  label: string
  clicks: number
  new_urls?: number
}

export interface TrendQuery {
  granularity?: TrendGranularity
  date_from?: string
  date_to?: string
  days?: number
}

export interface TopUrl {
  id: number
  uid: string
  long_url: string
  title: string | null
  clicks: number
}

export interface TopQuery {
  limit?: number
  date_from?: string
  date_to?: string
}

export interface RecentUrl {
  id: number
  uid: string
  long_url: string
  title: string | null
  clicks: number
  source: string
  created_at: string
}

/** 概览：总数 / 今日新增 / 今日点击 / 活跃率 */
export function getOverview(): Promise<StatsOverview> {
  return request.get<StatsOverview>('/stats/overview')
}

/** 点击趋势（按小时/天/月聚合） */
export function getTrend(params: TrendQuery): Promise<TrendPoint[]> {
  return request.get<TrendPoint[]>('/stats/trend', { params })
}

/** Top N 短链 */
export function getTop(params: TopQuery): Promise<TopUrl[]> {
  return request.get<TopUrl[]>('/stats/top', { params })
}

/** 最近创建列表 */
export function getRecent(limit = 20): Promise<RecentUrl[]> {
  return request.get<RecentUrl[]>('/stats/recent', { params: { limit } })
}
