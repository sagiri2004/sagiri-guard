import type { Metric } from '../../types/dashboard'
import MetricCard from './MetricCard'

type MetricsGridProps = {
  metrics: Metric[]
}

const MetricsGrid = ({ metrics }: MetricsGridProps) => (
  <section className="metrics-grid">
    {metrics.map((metric) => (
      <MetricCard key={metric.label} {...metric} />
    ))}
  </section>
)

export default MetricsGrid

