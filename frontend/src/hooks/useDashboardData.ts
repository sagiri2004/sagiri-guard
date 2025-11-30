import { useCallback, useSyncExternalStore } from 'react'

import { dashboardService } from '../services/api'
import { dashboardStore } from '../store/dashboardStore'

export function useDashboardData() {
  const data = useSyncExternalStore(dashboardStore.subscribe, dashboardStore.getState)

  const refresh = useCallback(async () => {
    const latest = await dashboardService.fetchDashboard()
    dashboardStore.setState(latest)
  }, [])

  return { data, refresh }
}

