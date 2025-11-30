import type { DashboardData } from '../types/dashboard'

export const dashboardData: DashboardData = {
  metrics: [
    { label: 'Agents online', value: '42', trend: '+5 vs last hour' },
    { label: 'Pending tasks', value: '12', trend: '3 due soon' },
    { label: 'Backup success', value: '98%', trend: '+1% this week' },
    { label: 'Open alerts', value: '7', trend: '2 critical' },
  ],
  alerts: [
    {
      id: 'INC-1432',
      summary: 'Unauthorized login attempt detected on SG-12',
      timestamp: 'Just now',
      severity: 'high',
    },
    {
      id: 'INC-1420',
      summary: 'Backup lag exceeded threshold for cluster-east',
      timestamp: '7 min ago',
      severity: 'medium',
    },
    {
      id: 'INC-1399',
      summary: 'Unapproved USB device blocked on SG-08',
      timestamp: '18 min ago',
      severity: 'medium',
    },
  ],
  tasks: [
    {
      id: 'TASK-210',
      title: 'Review backup schedule for finance nodes',
      owner: 'Operations',
      status: 'In progress',
    },
    {
      id: 'TASK-204',
      title: 'Ship new agent build to QA ring',
      owner: 'Release',
      status: 'Queued',
    },
    {
      id: 'TASK-198',
      title: 'Rotate privileged access keys',
      owner: 'Security',
      status: 'Blocked',
    },
  ],
  activity: [
    {
      id: 1,
      description: 'Agent SG-31 applied policy v2.8',
      time: '09:24',
    },
    {
      id: 2,
      description: 'New device enrollment approved by admin',
      time: '09:12',
    },
    {
      id: 3,
      description: 'Backup job #422 completed (42 GB)',
      time: '08:59',
    },
    {
      id: 4,
      description: 'Alert INC-1387 resolved by analyst',
      time: '08:41',
    },
  ],
}

