import type { PropsWithChildren } from 'react'

type MainLayoutProps = PropsWithChildren<{
  onLogout?: () => void
}>

const MainLayout = ({ children, onLogout }: MainLayoutProps) => (
  <div className="dashboard">
    <div className="topbar">
      <div className="brand">Sagiri Console</div>
      {onLogout && (
        <button className="logout-btn" onClick={onLogout}>
          Logout
        </button>
      )}
    </div>
    {children}
  </div>
)

export default MainLayout

