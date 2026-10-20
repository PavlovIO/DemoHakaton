import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { AppLayout } from '@/layouts/AppLayout'
import { PrivateRoute } from '@/components/PrivateRoute'
import { Login } from '@/pages/Login'
import { Dashboard } from '@/pages/Dashboard'
import { Applications } from '@/pages/Applications'
import { Offers } from '@/pages/Offers'
import { Deals } from '@/pages/Deals'

const router = createBrowserRouter([
  {
    path: '/login',
    element: <Login />,
  },
  {
    path: '/',
    element: (
      <PrivateRoute>
        <AppLayout />
      </PrivateRoute>
    ),
    children: [
      {
        index: true,
        element: <Dashboard />,
      },
      {
        path: 'dashboard',
        element: <Dashboard />,
      },
      {
        path: 'applications',
        element: <Applications />,
      },
      {
        path: 'offers',
        element: <Offers />,
      },
      {
        path: 'deals',
        element: <Deals />,
      },
    ],
  },
])

export function AppRouter() {
  return <RouterProvider router={router} />
}