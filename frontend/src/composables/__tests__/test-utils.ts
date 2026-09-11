/** 等待所有已排队的微任务执行完（无需挂载组件树） */
export function flushPromises(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0))
}
