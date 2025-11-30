import type { DashboardData } from '../types/dashboard'
import { dashboardData } from '../data/dashboard'

type Listener = () => void

let state: DashboardData = dashboardData
const listeners = new Set<Listener>()

export const dashboardStore = {
  getState: () => state,
  setState: (nextState: DashboardData) => {
    state = nextState
    listeners.forEach((listener) => listener())
  },
  subscribe: (listener: Listener) => {
    listeners.add(listener)
    return () => {
      listeners.delete(listener)
    }
  },
}

