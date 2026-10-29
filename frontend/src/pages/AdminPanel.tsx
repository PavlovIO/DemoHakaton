import { Navigate, Outlet } from 'react-router-dom'

// Layout панели администратора. Разделы — пункты верхнего меню (Navigation),
// своих вкладок у панели нет: они дублировали меню.
export function AdminPanel() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-gray-900">Панель администратора</h1>
      <Outlet />
    </div>
  )
}

// /admin без вложенного пути — на первую вкладку.
export function AdminIndexRedirect() {
  return <Navigate to="/admin/kyc" replace />
}
