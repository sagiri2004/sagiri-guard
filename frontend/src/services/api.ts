import type { DashboardData } from '../types/dashboard'
import { dashboardData } from '../data/dashboard'

export const dashboardService = {
  async fetchDashboard(): Promise<DashboardData> {
    await new Promise((resolve) => setTimeout(resolve, 350))
    return dashboardData
  },
}

