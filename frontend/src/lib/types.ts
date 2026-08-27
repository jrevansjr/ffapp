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

/** PlayerDraftData combines market cost with FantasyPros expert consensus. */
export interface PlayerDraftData {
  aggregate_adp: number | null
  ecr: number | null
  position_rank: number | null
  tier: number | null
  rank_min: number | null
  rank_max: number | null
  rank_std_dev: number | null
}

export interface PlayerSeasonStats {
  season: number
  games_played: number
  fantasy_points_half_ppr: number
  average_fantasy_points: number | null
  passing_yards: number
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
  draft: PlayerDraftData
  season: PlayerSeasonStats | null
  odds: PlayerOdds
  is_taken: boolean
}

/** PlayerFilters are local presentation state; they do not trigger API calls. */
export interface PlayerFilters {
  position: string
  team: string
}

export interface PlayerProviderIDs {
  gsis: string | null
  fantasypros: string | null
  espn: string | null
  sportradar: string | null
  rotowire: string | null
  rotoworld: string | null
  yahoo: string | null
  fantasy_data: string | null
  stats: string | null
}

/** PlayerProfile is the identity and status portion of a player-detail response. */
export interface PlayerProfile {
  id: number
  sleeper_player_id: string | null
  first_name: string
  last_name: string
  position: string
  nfl_team: NFLTeam | null
  birth_date: string | null
  age: number | null
  active: boolean
  status: string | null
  number: number | null
  college: string | null
  height: string | null
  weight: string | null
  birth_country: string | null
  years_exp: number | null
  depth_chart_position: string | null
  depth_chart_order: number | null
  injury_status: string | null
  injury_start_date: string | null
  practice_participation: string | null
  provider_ids: PlayerProviderIDs
  is_taken: boolean
}

export interface OddsLine {
  season: number
  source: string
  market: string
  line: number
  over_price: number | null
  under_price: number | null
  captured_at: string
}

export interface PlayerDetailOdds {
  touchdowns: OddsLine | null
  team_wins: OddsLine | null
}

/** PlayerWeekStats is one persisted game-week used by the inspector charts. */
export interface PlayerWeekStats {
  season: number
  week: number
  fantasy_points_half_ppr: number
  passing_yards: number
  targets: number
  receptions: number
  rushing_attempts: number
  receiving_yards: number
  rushing_yards: number
  receiving_touchdowns: number
  rushing_touchdowns: number
}

export interface WeeklySummary {
  average: number | null
  high: number | null
  median: number | null
  low: number | null
}

/** PlayerDetail is the complete local payload used by the Draft Day inspector. */
export interface PlayerDetail {
  player: PlayerProfile
  season: PlayerSeasonStats | null
  draft: PlayerDraftData
  odds: PlayerDetailOdds
  weekly: PlayerWeekStats[]
  weekly_summary: WeeklySummary
}
