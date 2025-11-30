import type { ActivityItem } from '../../types/dashboard'

type ActivityPanelProps = {
  items: ActivityItem[]
}

const ActivityPanel = ({ items }: ActivityPanelProps) => (
  <article className="card panel full">
    <header>
      <h2>Latest Activity</h2>
      <span className="badge">{items.length}</span>
    </header>
    <ul className="activity-list">
      {items.map(({ id, description, time }) => (
        <li key={id}>
          <span>{description}</span>
          <span className="activity-time">{time}</span>
        </li>
      ))}
    </ul>
  </article>
)

export default ActivityPanel

