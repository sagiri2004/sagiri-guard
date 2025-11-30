import type { Alert } from '../../types/dashboard'
import { classNames } from '../../utils/classNames'

type AlertsPanelProps = {
  alerts: Alert[]
}

const AlertsPanel = ({ alerts }: AlertsPanelProps) => (
  <article className="card panel">
    <header>
      <h2>Active Alerts</h2>
      <span className="badge">{alerts.length}</span>
    </header>
    <ul>
      {alerts.map(({ id, summary, timestamp, severity }) => (
        <li key={id}>
          <div className="item-head">
            <p className="item-title">{summary}</p>
            <span className={classNames('severity', severity)}>{severity}</span>
          </div>
          <p className="item-note">
            {id} • {timestamp}
          </p>
        </li>
      ))}
    </ul>
  </article>
)

export default AlertsPanel

