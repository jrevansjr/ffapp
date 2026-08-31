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

  let label = "Checking local API…"
  let indicatorClass = "bg-neutral-400"

  if (health.isError) {
    label = "Local API unavailable"
    indicatorClass = "bg-red-600"
  } else if (health.data) {
    label = "Local API connected"
    indicatorClass = "bg-emerald-600"
  }

  return (
    <div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground sm:text-sm" role="status">
      <span className={cn("size-2 rounded-full", indicatorClass)} aria-hidden="true" />
      <span>{label}</span>
    </div>
  )
}

function Layout() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border bg-card">
        <div className="mx-auto flex max-w-screen-2xl flex-wrap items-center justify-between gap-x-6 gap-y-2 px-4 py-3 sm:px-6">
          <div className="flex min-w-0 flex-wrap items-center gap-x-6 gap-y-2">
            <span className="shrink-0 font-semibold">Fantasy Football Draft</span>
            <nav aria-label="Primary navigation">
              <ul className="flex flex-wrap items-center gap-1">
                {navigation.map((item) => (
                  <li key={item.to}>
                    <NavLink
                      className={({ isActive }) =>
                        cn(
                          "block rounded-md px-2.5 py-1.5 text-sm font-medium text-muted-foreground hover:bg-muted hover:text-foreground sm:px-3",
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
      <main className="mx-auto max-w-screen-2xl px-4 py-5 sm:px-6 sm:py-6">
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
