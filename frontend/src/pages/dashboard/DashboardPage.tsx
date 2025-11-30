import MetricsGrid from '../../components/metrics/MetricsGrid'
import AlertsPanel from '../../components/panels/AlertsPanel'
import TasksPanel from '../../components/panels/TasksPanel'
import ActivityPanel from '../../components/panels/ActivityPanel'
import { useDashboardData } from '../../hooks/useDashboardData'

const DashboardPage = () => {
  const { data, refresh } = useDashboardData()

  return (
    <>
      <header className="page-header">
        <div>
          <p className="eyebrow">Monitoring Console</p>
          <h1>Sagiri Guard Dashboard</h1>
          <p className="subtitle">
            React + Vite + TypeScript scaffold ready for connecting to the backend API.
            Use these sample widgets as placeholders while wiring real data.
          </p>
        </div>
        <button className="primary-btn" type="button" onClick={refresh}>
          Refresh data
        </button>
      </header>

      <MetricsGrid metrics={data.metrics} />

      <section className="panels-grid">
        <AlertsPanel alerts={data.alerts} />
        <TasksPanel tasks={data.tasks} />
        <ActivityPanel items={data.activity} />
      </section>
    </>
  )
}

export default DashboardPage

