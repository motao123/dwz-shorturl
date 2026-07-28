import request from './request'

export interface Domain {
  id: number
  domain: string
  scheme: string
  name: string
  project: string
  status: number  // 1=启用 0=停用 2=异常
  priority: number
  dns_status: string  // pending/ok/fail
  ssl_status: string  // pending/ok/fail
  link_count: number
  created_at: string
  updated_at: string
}

export interface DomainPayload {
  domain: string
  scheme?: string
  name?: string
  project?: string
  priority?: number
  status?: number
}

export interface ActiveDomain {
  id: number
  domain: string
  scheme: string
  name: string
  priority: number
}

export function listDomains(status?: number): Promise<Domain[]> {
  return request.get<Domain[]>('/domains', { params: status !== undefined ? { status } : {} })
}
export function getDomain(id: number): Promise<Domain> {
  return request.get<Domain>(`/domains/${id}`)
}
export function createDomain(data: DomainPayload): Promise<Domain> {
  return request.post<Domain>('/domains', data)
}
export function updateDomain(id: number, data: DomainPayload): Promise<Domain> {
  return request.put<Domain>(`/domains/${id}`, data)
}
export function deleteDomain(id: number): Promise<null> {
  return request.delete<null>(`/domains/${id}`)
}
export function checkDomain(id: number): Promise<null> {
  return request.post<null>(`/domains/${id}/check`)
}
export function batchUpdateStatus(ids: number[], status: number): Promise<null> {
  return request.put<null>('/domains/batch-status', { ids, status })
}
export function getActiveDomains(): Promise<ActiveDomain[]> {
  return request.get<ActiveDomain[]>('/domains/active')
}
