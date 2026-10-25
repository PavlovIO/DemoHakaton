import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  hasError: boolean
  error?: Error
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[ErrorBoundary]', error, info)
  }

  render() {
    if (this.state.hasError) {
      return (
        this.props.fallback ?? (
          <div className="flex min-h-screen items-center justify-center p-6">
            <div className="max-w-md rounded-lg border border-red-200 bg-red-50 p-6 text-center">
              <h2 className="text-lg font-semibold text-red-800">Что-то сломалось</h2>
              <p className="mt-2 text-sm text-red-700">
                {this.state.error?.message ?? 'Неизвестная ошибка'}
              </p>
              <button
                onClick={() => window.location.reload()}
                className="mt-4 rounded bg-red-600 px-4 py-2 text-sm text-white hover:bg-red-700"
              >
                Обновить страницу
              </button>
            </div>
          </div>
        )
      )
    }
    return this.props.children
  }
}