import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User } from '../api/types'

interface AuthState {
  token: string | null
  user: User | null
  setAuth: (token: string, user: User) => void
  clear: () => void
}

export const useAuth = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      setAuth: (token, user) => set({ token, user }),
      clear: () => set({ token: null, user: null }),
    }),
    { name: 'service-edge-auth' },
  ),
)
