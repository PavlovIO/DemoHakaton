import { useEffect, useState } from 'react'

export const SEARCH_DEBOUNCE_MS = 300

// Значение «успокаивается» через delay после последнего изменения: поиск по
// пользователям не уходит на сервер на каждый нажатый символ.
export function useDebounce<T>(value: T, delay = SEARCH_DEBOUNCE_MS): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])

  return debounced
}
