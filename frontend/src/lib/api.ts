export interface HealthResponse {
  status: string
}

export async function getHealth(): Promise<HealthResponse> {
  const response = await fetch("/api/health")
  if (!response.ok) {
    throw new Error("Backend health check failed")
  }

  return response.json() as Promise<HealthResponse>
}

