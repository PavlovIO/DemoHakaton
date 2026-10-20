/* eslint-disable react-refresh/only-export-components */

import { createContext, useContext, useState, type ReactNode } from 'react'

export type UserRole = 'borrower' | 'lender' | 'admin'

export interface User {
  id: string
  email: string
  role: UserRole
  organizationName: string
  kycStatus: 'draft' | 'under_review' | 'verified' | 'rejected'
}

interface AuthContextType {
  user: User | null
  login: (email: string, password: string) => Promise<void>
  logout: () => void
  isLoading: boolean
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

// Моковые данные для демонстрации
const mockUsers: Record<UserRole, User> = {
  borrower: {
    id: '1',
    email: 'borrower@alpha.ru',
    role: 'borrower',
    organizationName: 'ООО "Альфа"',
    kycStatus: 'verified',
  },
  lender: {
    id: '2',
    email: 'lender@beta.com',
    role: 'lender',
    organizationName: 'Beta Corp',
    kycStatus: 'verified',
  },
  admin: {
    id: '3',
    email: 'admin@platform.com',
    role: 'admin',
    organizationName: 'Platform Admin',
    kycStatus: 'verified',
  },
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  const login = async (email: string, _password: string) => {
    setIsLoading(true)
    // Имитация задержки API
    await new Promise((resolve) => setTimeout(resolve, 500))

    // Определяем роль по email (для демо)
    let role: UserRole = 'borrower'
    if (email.includes('lender')) role = 'lender'
    if (email.includes('admin')) role = 'admin'

    setUser(mockUsers[role])
    setIsLoading(false)
  }

  const logout = () => {
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, login, logout, isLoading }}>
      { children }
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}