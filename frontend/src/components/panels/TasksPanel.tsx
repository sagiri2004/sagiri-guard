import type { TaskItem } from '../../types/dashboard'

type TasksPanelProps = {
  tasks: TaskItem[]
}

const TasksPanel = ({ tasks }: TasksPanelProps) => (
  <article className="card panel">
    <header>
      <h2>Operations Queue</h2>
      <span className="badge">{tasks.length}</span>
    </header>
    <ul>
      {tasks.map(({ id, title, owner, status }) => (
        <li key={id}>
          <div className="item-head">
            <p className="item-title">{title}</p>
            <span className="status">{status}</span>
          </div>
          <p className="item-note">
            {id} • {owner}
          </p>
        </li>
      ))}
    </ul>
  </article>
)

export default TasksPanel

