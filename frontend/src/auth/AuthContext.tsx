import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { api, clearToken, getToken, setToken, setUnauthorizedHandler } from '../api/client'
import type { LoginResponse, Role } from '../api/types'

interface AuthState {
  token: string | null
  role: Role | null
}

interface AuthContextValue extends AuthState {
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const ROLE_KEY = 'ts_role'

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(() => ({
    token: getToken(),
    role: (localStorage.getItem(ROLE_KEY) as Role | null) ?? null,
  }))

  useEffect(() => {
    setUnauthorizedHandler(() => {
      localStorage.removeItem(ROLE_KEY)
      setState({ token: null, role: null })
    })
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const resp = await api.post<LoginResponse>('/api/login', { username, password })
    setToken(resp.token)
    localStorage.setItem(ROLE_KEY, resp.role)
    setState({ token: resp.token, role: resp.role })
  }, [])

  const logout = useCallback(async () => {
    try {
      await api.post('/api/logout')
    } finally {
      clearToken()
      localStorage.removeItem(ROLE_KEY)
      setState({ token: null, role: null })
    }
  }, [])

  return <AuthContext.Provider value={{ ...state, login, logout }}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return ctx
}
