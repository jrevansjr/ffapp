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

/** NFLTeam is the local team identity included with player summaries. */
export interface NFLTeam {
  id: number
  abbreviation: string
  name: string
}

export interface PlayerADP {
  fantasypros: number | null
  sleeper: number | null
  underdog: number | null
}

export interface PlayerSeasonStats {
  season: number
  games_played: number
  fantasy_points_half_ppr: number
  average_fantasy_points: number | null
  targets: number
  targets_per_game: number | null
  receptions: number
  rushing_attempts: number
  rushing_attempts_per_game: number | null
  receiving_yards: number
  rushing_yards: number
  receiving_touchdowns: number
  rushing_touchdowns: number
}

export interface PlayerOdds {
  touchdown_line: number | null
  team_win_line: number | null
}

/** PlayerListItem matches the compact, persisted-data DTO used by Overview. */
export interface PlayerListItem {
  id: number
  sleeper_player_id: string | null
  first_name: string
  last_name: string
  position: string
  nfl_team: NFLTeam | null
  age: number | null
  number: number | null
  years_exp: number | null
  depth_chart_position: string | null
  depth_chart_order: number | null
  injury_status: string | null
  adp: PlayerADP
  season: PlayerSeasonStats | null
  odds: PlayerOdds
  tier: number | null
  is_taken: boolean
}

/** PlayerFilters are local presentation state; they do not trigger API calls. */
export interface PlayerFilters {
  position: string
  team: string
}
