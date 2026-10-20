function App() {
  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <header className="border-b border-gray-200 bg-white">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          <div className="flex items-center gap-8">
            <h1 className="text-xl font-bold text-primary-700">
              Liquidity Platform
            </h1>
            <nav className="hidden md:flex items-center gap-6">
              <a href="#" className="text-sm font-medium text-gray-900 hover:text-primary-600">
                Мои заявки
              </a>
              <a href="#" className="text-sm font-medium text-gray-500 hover:text-primary-600">
                Сделки
              </a>
              <a href="#" className="text-sm font-medium text-gray-500 hover:text-primary-600">
                Верификация
              </a>
            </nav>
          </div>
          <div className="flex items-center gap-4">
            <button className="relative text-gray-500 hover:text-gray-700">
              🔔
              <span className="absolute -top-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-danger text-[10px] font-bold text-white">
                3
              </span>
            </button>
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 text-sm font-semibold text-primary-700">
              А
            </div>
          </div>
        </div>
      </header>

      {/* Main content */}
      <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
          <h2 className="text-lg font-semibold text-gray-900">
            Добро пожаловать на платформу ликвидности
          </h2>
          <p className="mt-2 text-sm text-gray-600">
            Tailwind CSS v4 успешно подключён. Движок на Rust, конфигурация через @theme.
          </p>
          <div className="mt-6 flex gap-3">
            <span className="status-draft inline-flex items-center rounded-full px-3 py-1 text-xs font-medium">
              Черновик
            </span>
            <span className="status-published inline-flex items-center rounded-full px-3 py-1 text-xs font-medium">
              Опубликована
            </span>
            <span className="status-agreed inline-flex items-center rounded-full px-3 py-1 text-xs font-medium">
              Согласована
            </span>
            <span className="status-rejected inline-flex items-center rounded-full px-3 py-1 text-xs font-medium">
              Отклонена
            </span>
          </div>
        </div>
      </main>
    </div>
  )
}

export default App
