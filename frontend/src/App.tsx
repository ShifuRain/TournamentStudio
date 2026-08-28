import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { queryClient } from './api/queryClient'
import { AuthProvider } from './auth/AuthContext'
import { ProtectedRoute } from './auth/ProtectedRoute'
import { AppShell } from './components/AppShell'
import { LoginPage } from './pages/LoginPage'
import { TournamentListPage } from './pages/TournamentListPage'
import { TournamentCreatePage } from './pages/TournamentCreatePage'
import { TournamentDetailPage } from './pages/TournamentDetailPage'
import { TeamsTab } from './pages/TeamsTab'
import { TeamImportPage } from './pages/TeamImportPage'
import { SchedulePage } from './pages/SchedulePage'
import { WatchPage } from './pages/WatchPage'
import { PluginsPage } from './pages/PluginsPage'

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route element={<ProtectedRoute />}>
              <Route element={<AppShell />}>
                <Route path="/" element={<Navigate to="/tournaments" replace />} />
                <Route path="/tournaments" element={<TournamentListPage />} />
                <Route path="/plugins" element={<PluginsPage />} />
                <Route path="/tournaments/new" element={<TournamentCreatePage />} />
                <Route path="/tournaments/:id" element={<TournamentDetailPage />}>
                  <Route index element={<Navigate to="teams" replace />} />
                  <Route path="teams" element={<TeamsTab />} />
                  <Route path="teams/import" element={<TeamImportPage />} />
                  <Route path="schedule" element={<SchedulePage />} />
                  <Route path="watch" element={<WatchPage />} />
                </Route>
              </Route>
            </Route>
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </QueryClientProvider>
  )
}

export default App
