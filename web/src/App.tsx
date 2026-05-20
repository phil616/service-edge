import { Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from './store/auth'
import AppLayout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import FRPSList from './pages/FRPSList'
import FRPSNew from './pages/FRPSNew'
import FRPSDetail from './pages/FRPSDetail'
import FRPCList from './pages/FRPCList'
import FRPCNew from './pages/FRPCNew'
import FRPCDetail from './pages/FRPCDetail'
import AuditLogs from './pages/AuditLogs'
import Settings from './pages/Settings'
import Help from './pages/Help'
import Topology from './pages/Topology'

function RequireAuth({ children }: { children: JSX.Element }) {
  const token = useAuth((s) => s.token)
  if (!token) return <Navigate to="/login" replace />
  return children
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="topology" element={<Topology />} />
        <Route path="frps" element={<FRPSList />} />
        <Route path="frps/new" element={<FRPSNew />} />
        <Route path="frps/:uuid" element={<FRPSDetail />} />
        <Route path="frpc" element={<FRPCList />} />
        <Route path="frpc/new" element={<FRPCNew />} />
        <Route path="frpc/:uuid" element={<FRPCDetail />} />
        <Route path="audit-logs" element={<AuditLogs />} />
        <Route path="settings" element={<Settings />} />
        <Route path="help" element={<Help />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
