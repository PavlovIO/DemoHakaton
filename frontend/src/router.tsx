import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { AppLayout } from '@/layouts/AppLayout'
import { PrivateRoute } from '@/components/PrivateRoute'
import { Login } from '@/pages/Login'
import { Register } from '@/pages/Register'
import { Dashboard } from '@/pages/Dashboard'
import { Applications } from '@/pages/Applications'
import { Offers } from '@/pages/Offers'
import { Deals } from '@/pages/Deals'
import { KycStub } from '@/pages/KycStub'
import { ProfileStub } from '@/pages/ProfileStub'
import { Marketplace } from '@/pages/Marketplace'
import { AdminPanel, AdminIndexRedirect } from '@/pages/AdminPanel'
import { KycQueueTab } from '@/pages/admin/KycQueue'
import { ApplicationsModerationTab } from '@/pages/admin/ApplicationsModerationTab'
import { AmlFlagsTab } from '@/pages/admin/AmlFlagsTab'
import { UsersTab } from '@/pages/admin/UsersTab'
import { AnalyticsTab } from '@/pages/admin/AnalyticsTab'

const router = createBrowserRouter([
  { path: '/login', element: <Login /> },
  { path: '/register', element: <Register /> },
  {
    path: '/profile',
    element: (
      <PrivateRoute requireKyc={false}>
        <AppLayout />
      </PrivateRoute>
    ),
    children: [
      { index: true, element: <ProfileStub /> },
      { path: 'kyc', element: <KycStub /> },
    ],
  },
  {
    path: '/admin',
    element: (
      <PrivateRoute requireKyc={false} requireRole="admin">
        <AppLayout />
      </PrivateRoute>
    ),
    children: [
      {
        element: <AdminPanel />,
        children: [
          { index: true, element: <AdminIndexRedirect /> },
          // Вкладки — отдельные маршруты. Пункты левого меню ведут сюда.
          { path: 'kyc', element: <KycQueueTab /> },
          { path: 'applications', element: <ApplicationsModerationTab /> },
          { path: 'aml', element: <AmlFlagsTab /> },
          { path: 'users', element: <UsersTab /> },
          { path: 'analytics', element: <AnalyticsTab /> },
          // Неизвестный путь внутри /admin — редирект на первую вкладку.
          { path: '*', element: <AdminIndexRedirect /> },
        ],
      },
    ],
  },
  {
    path: '/',
    element: (
      <PrivateRoute>
        <AppLayout />
      </PrivateRoute>
    ),
    children: [
      { index: true, element: <Dashboard /> },
      { path: 'dashboard', element: <Dashboard /> },
      { path: 'applications', element: <Applications /> },
      { path: 'applications/:id', element: <div>Детали заявки</div> },
      { path: 'marketplace', element: <Marketplace /> },
      { path: 'offers', element: <Offers /> },
      { path: 'deals', element: <Deals /> },
    ],
  },
])

export function AppRouter() {
  return <RouterProvider router={router} />
}