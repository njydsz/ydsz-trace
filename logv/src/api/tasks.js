import http from './http'

// listTasks 分页查询检索任务列表。
// params: { page, pageSize, status? }
export function listTasks(params = {}) {
  return http.get('/logs/tasks', { params })
}

// getTask 按 taskNo 查询单条任务详情。
export function getTask(taskNo) {
  return http.get(`/logs/tasks/${encodeURIComponent(taskNo)}`)
}

// retryTask 创建一条重试任务，返回新 taskNo。
export function retryTask(taskNo) {
  return http.post(`/logs/tasks/${encodeURIComponent(taskNo)}/retry`)
}

// deleteTask 删除一条任务记录。
export function deleteTask(taskNo) {
  return http.delete(`/logs/tasks/${encodeURIComponent(taskNo)}`)
}

// 任务状态常量。
export const TASK_STATUS = {
  PENDING: 'pending',
  RUNNING: 'running',
  SUCCESS: 'success',
  FAILED: 'failed',
  PURGED: 'purged',
}
