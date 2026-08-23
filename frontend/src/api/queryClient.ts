import { QueryClient } from '@tanstack/react-query'

// Module-scope singleton shared between App.tsx (which provides it to
// the React tree) and AuthContext.tsx (which clears it on logout and
// on an auto-logout triggered by a 401) -- see queryClient.clear()
// calls in AuthContext for why: without this, a cleared token still
// left cached query data (e.g. ['tournaments'], ['teams', id]) served
// to whoever logs in next in the same browser tab.
export const queryClient = new QueryClient()
