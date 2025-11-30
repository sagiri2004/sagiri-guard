import type { PropsWithChildren } from 'react'

const MainLayout = ({ children }: PropsWithChildren) => (
  <div className="dashboard">{children}</div>
)

export default MainLayout

