import request, { type PageParams, type PageResult } from './request'

export interface WebhookSub {
  id: number
  name: string
  url: string
  events: string[]
  status: number
  created_at: string
}

export interface WebhookPayload {
  name: string
  url: string
  events: string[]
  secret?: string
}

export interface WebhookDelivery {
  id: number
  webhook_id: number
  event: string
  payload: string
  response_status: number
  response_body: string
  attempt: number
  success: number
  created_at: string
}

export interface DeliveryParams extends PageParams {
  webhook_id?: number
  event?: string
  result?: 'success' | 'failed'
}

/** Webhook 列表 */
export function listWebhooks(): Promise<WebhookSub[]> {
  return request.get<WebhookSub[]>('/webhooks')
}

/** 创建 Webhook */
export function createWebhook(data: WebhookPayload): Promise<WebhookSub> {
  return request.post<WebhookSub>('/webhooks', data)
}

/** 删除 Webhook */
export function removeWebhook(id: number): Promise<null> {
  return request.delete<null>(`/webhooks/${id}`)
}

/** 投递记录列表 */
export function listWebhookDeliveries(params: DeliveryParams): Promise<PageResult<WebhookDelivery>> {
  return request.get<PageResult<WebhookDelivery>>('/webhooks/deliveries', { params })
}

/** 测试 Webhook（发送 ping 事件） */
export function pingWebhook(id: number): Promise<WebhookDelivery> {
  return request.post<WebhookDelivery>(`/webhooks/${id}/ping`)
}

/** 重试投递记录 */
export function retryWebhookDelivery(id: number): Promise<WebhookDelivery> {
  return request.post<WebhookDelivery>(`/webhooks/deliveries/${id}/retry`)
}

export const WEBHOOK_EVENTS = [
  { value: 'link.created', label: '短链创建 (link.created)' },
  { value: 'link.clicked', label: '短链点击 (link.clicked)' }
]