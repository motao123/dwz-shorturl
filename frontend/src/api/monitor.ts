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
  /** 任务最近一次执行是否失败（当前仅分区维护任务上报） */
  failed?: boolean
  error?: string
  consecutive_failures?: number
}

/** click_logs 月度分区覆盖与维护健康状态 */
export interface PartitionStatus {
  table: string
  /** 表存在且按 RANGE 分区 */
  available: boolean
  months: string[]
  earliest_month: string
  latest_month: string
  /** 必须覆盖到的最远月份（当前月 + ahead 个月） */
  required_month: string
  /** 落后目标月的月数，0 表示已达标 */
  behind_months: number
  /** 下一次维护将创建的月份 */
  pending_months: string[] | null
  future_rows: number
  total_rows: number
  /** p_future 行数占全表比例，0~1 */
  future_ratio: number
  has_future: boolean
  /** 结构异常：table_not_partitioned / future_partition_missing */
  mismatch?: string
  healthy: boolean
  last_error?: string
  last_error_at?: string
  last_run_at?: string
  created_last_run: number
  consecutive_failures: number
  failing: boolean
  /** ok / warning / critical */
  alert_level: 'ok' | 'warning' | 'critical'
  alerts?: string[] | null
}

/** 手动补齐分区的结果（支持部分成功） */
export interface PartitionEnsureResult {
  created: string[]
  pending: string[] | null
  dry_run: boolean
  failed_month?: string
  error?: string
  status?: PartitionStatus
  alerts?: string[] | null
  alert_level?: string
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

/**
 * 手动补齐 click_logs 月度分区（幂等，months=0 时使用后端默认 2 个月）。
 * dry_run=true 时只返回将创建的月份，不加 DDL 锁。
 */
export function ensurePartitions(months = 0, dryRun = false): Promise<PartitionEnsureResult> {
  return request.post<PartitionEnsureResult>('/monitor/ensure-partitions', null, {
    params: { months, dry_run: dryRun }
  })
}

/** click_logs 分区覆盖状态（只读，不触发 DDL） */
export function getPartitionStatus(): Promise<PartitionStatus> {
  return request.get<PartitionStatus>('/monitor/partitions')
}
