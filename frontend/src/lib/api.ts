import type { Settings, SettingsUpdate } from "@/lib/types"

export interface HealthResponse {
  status: string
}

interface ErrorResponse {
  error?: unknown
}

/** Reads a safe backend error message without exposing response internals. */
async function responseError(response: Response, fallback: string): Promise<Error> {
  try {
    const body = (await response.json()) as ErrorResponse
    if (typeof body.error === "string" && body.error !== "") {
      return new Error(body.error)
    }
  } catch {
    // Non-JSON failures still receive the operation-specific fallback below.
  }
  return new Error(fallback)
}

export async function getHealth(): Promise<HealthResponse> {
  const response = await fetch("/api/health")
  if (!response.ok) {
    throw new Error("Backend health check failed")
  }

  return response.json() as Promise<HealthResponse>
}

/** Loads the persistent application settings through the Vite /api proxy. */
export async function getSettings(): Promise<Settings> {
  const response = await fetch("/api/settings")
  if (!response.ok) {
    throw await responseError(response, "Could not load settings")
  }

  return response.json() as Promise<Settings>
}

/** Replaces all user-editable settings and returns the saved database row. */
export async function updateSettings(settings: SettingsUpdate): Promise<Settings> {
  const response = await fetch("/api/settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(settings),
  })
  if (!response.ok) {
    throw await responseError(response, "Could not save settings")
  }

  return response.json() as Promise<Settings>
}
