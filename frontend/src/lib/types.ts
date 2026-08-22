/** Settings is the complete singleton configuration returned by the Go API. */
export interface Settings {
  sleeper_username: string
  sleeper_league_id: string
  sleeper_draft_id: string
  polling_enabled: boolean
  polling_interval_ms: number
  players_synced_at: string | null
  updated_at: string
}

/** SettingsUpdate contains only the fields the Admin page is allowed to edit. */
export interface SettingsUpdate {
  sleeper_username: string
  sleeper_league_id: string
  sleeper_draft_id: string
  polling_enabled: boolean
  polling_interval_ms: number
}
