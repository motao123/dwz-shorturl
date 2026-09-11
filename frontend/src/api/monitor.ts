import request from './request'

export interface DBStatus {
  healthy: boolean
  open_conns: number
  in_use: number
  idle: number
  error?: string
}

export interface RedisStatus {
  healthy: boolean
  error?: string
}

export interface QueueStatus {
  pending: number
}

export interface CronStatus {
  name: string
  last_run: string
}

/** click_logs 月度分区覆盖与维护健康状态 */
export interface PartitionStatus {
  table: string
  months: string[]
  earliest_month: string
  latest_month: string
  required_month: string
  behind_months: number
  future_rows: number
  healthy: boolean
  last_error?: string
  last_error_at?: string
  last_run_at?: string
  created_last_run: number
  consecutive_failures: number
  failing: boolean
}

export interface MonitorStatus {
  uptime: string
  start_time: string
  goroutines: number
  db: DBStatus
  redis: RedisStatus
  queue: QueueStatus
  cron: CronStatus[]
  partitions?: PartitionStatus
}

/** 系统监控状态 */
export function getMonitorStatus(): Promise<MonitorStatus> {
  return request.get<MonitorStatus>('/monitor')
}

/** 手动补齐 click_logs 月度分区（months=0 时使用后端默认 2 个月） */
export function ensurePartitions(months = 0): Promise<{ created: number }> {
  return request.post<{ created: number }>('/monitor/ensure-partitions', null, {
    params: { months }
  })
}
