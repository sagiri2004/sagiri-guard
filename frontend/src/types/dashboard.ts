export type Metric = {
  label: string
  value: string
  trend: string
}

export type Alert = {
  id: string
  summary: string
  timestamp: string
  severity: 'high' | 'medium' | 'low'
}

export type TaskItem = {
  id: string
  title: string
  owner: string
  status: 'Queued' | 'In progress' | 'Blocked' | 'Done'
}

export type ActivityItem = {
  id: number
  description: string
  time: string
}

export type DashboardData = {
  metrics: Metric[]
  alerts: Alert[]
  tasks: TaskItem[]
  activity: ActivityItem[]
}

