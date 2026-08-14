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

export interface MonitorStatus {
  uptime: string
  start_time: string
  goroutines: number
  db: DBStatus
  redis: RedisStatus
  queue: QueueStatus
  cron: CronStatus[]
}

/** 系统监控状态 */
export function getMonitorStatus(): Promise<MonitorStatus> {
  return request.get<MonitorStatus>('/monitor')
}