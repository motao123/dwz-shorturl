import request from './request'

/** 配置值类型 */
export type ConfigValueType = 'string' | 'int' | 'bool' | 'json'

export interface ConfigItem {
  id: number
  config_key: string
  config_value: string
  value_type: ConfigValueType
  description: string | null
  is_public: 0 | 1
  updated_by: number | null
  updated_at: string
}

export interface ConfigUpdateItem {
  config_key: string
  config_value: string
}

/** 获取全部配置 */
export function getAllConfigs(): Promise<ConfigItem[]> {
  return request.get<ConfigItem[]>('/configs')
}

/** 批量更新配置 */
export function batchUpdateConfigs(items: ConfigUpdateItem[]): Promise<{ updated: number }> {
  return request.put<{ updated: number }>('/configs', { items })
}
