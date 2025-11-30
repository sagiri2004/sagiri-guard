import type { Metric } from '../../types/dashboard'

type MetricCardProps = Metric

const MetricCard = ({ label, value, trend }: MetricCardProps) => (
  <article className="card metric-card">
    <span className="label">{label}</span>
    <p className="value">{value}</p>
    <span className="trend">{trend}</span>
  </article>
)

export default MetricCard

