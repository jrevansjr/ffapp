import { lazy, Suspense } from "react"
import { useQuery } from "@tanstack/react-query"
import { Navigate, NavLink, Outlet, Route, Routes } from "react-router"

import { getHealth } from "@/lib/api"
import { cn } from "@/lib/utils"
import AdminPage from "@/pages/admin"
import OverviewPage from "@/pages/overview"

// Draft Day owns the charting dependency, so loading its route on demand keeps
// the lighter Overview and Admin pages out of that bundle.
const DraftPage = lazy(() => import("@/pages/draft"))

const navigation = [
  { label: "Overview", to: "/overview" },
  { label: "Draft Day", to: "/draft" },
  { label: "Admin", to: "/admin" },
]

function BackendStatus() {
  const health = useQuery({
    queryKey: ["health"],
    queryFn: getHealth,
    retry: false,
  })

  let label = "Checking backend…"
  let indicatorClass = "bg-neutral-400"

  if (health.isError) {
    label = "Backend unavailable"
    indicatorClass = "bg-red-600"
  } else if (health.data) {
    label = `Backend: ${health.data.status}`
    indicatorClass = "bg-emerald-600"
  }

  return (
    <div className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
      <span className={cn("size-2 rounded-full", indicatorClass)} aria-hidden="true" />
      <span>{label}</span>
    </div>
  )
}

function Layout() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border bg-card">
        <div className="mx-auto flex max-w-screen-2xl items-center justify-between gap-6 px-6 py-3">
          <div className="flex items-center gap-8">
            <span className="font-semibold">Fantasy Football Draft</span>
            <nav aria-label="Primary navigation">
              <ul className="flex items-center gap-1">
                {navigation.map((item) => (
                  <li key={item.to}>
                    <NavLink
                      className={({ isActive }) =>
                        cn(
                          "block rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground hover:bg-muted hover:text-foreground",
                          isActive && "bg-muted text-foreground",
                        )
                      }
                      to={item.to}
                    >
                      {item.label}
                    </NavLink>
                  </li>
                ))}
              </ul>
            </nav>
          </div>
          <BackendStatus />
        </div>
      </header>
      <main className="mx-auto max-w-screen-2xl px-6 py-6">
        <Outlet />
      </main>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Navigate replace to="/overview" />} />
        <Route path="overview" element={<OverviewPage />} />
        <Route
          path="draft"
          element={
            <Suspense
              fallback={
                <p className="text-sm text-muted-foreground" role="status">
                  Loading Draft Day…
                </p>
              }
            >
              <DraftPage />
            </Suspense>
          }
        />
        <Route path="admin" element={<AdminPage />} />
      </Route>
    </Routes>
  )
}
