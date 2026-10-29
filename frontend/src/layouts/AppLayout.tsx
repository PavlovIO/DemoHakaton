import { Outlet } from 'react-router-dom'
import { Header } from '@/components/Header'

export function AppLayout() {
  return (
    <div className="min-h-screen">
      <Header />
      <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        <Outlet />
      </main>
      <footer className="border-t border-green-300 bg-chrome">
        <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
          <p className="text-center text-sm text-gray-700">
            © 2026 Liquidity Platform. Все права защищены.
          </p>
        </div>
      </footer>
    </div>
  )
}