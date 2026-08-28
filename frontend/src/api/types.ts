export interface Tournament {
  id: number
  name: string
  sport_plugin_id: string
  tournament_type_id: string
  language: string
  status: string
}

export interface Team {
  id: number
  tournament_id: number
  name: string
  club: string
  extra_fields: Record<string, string>
}

export interface RosterField {
  key: string
  label: string
  required: boolean
}

export interface SportPlugin {
  id: string
  display_name: string
  compatible_tournament_types: string[]
  roster_fields: RosterField[] | null
}

export interface TournamentTypePlugin {
  id: string
  compatible_sports: string[]
}

export interface PluginsResponse {
  sports: SportPlugin[]
  tournament_types: TournamentTypePlugin[]
}

export type Role = 'organizer' | 'time_entry' | 'spectator'

export interface LoginResponse {
  token: string
  role: Role
}

export interface ImportProblem {
  row_index: number
  message: string
}

export interface ImportResult {
  imported: number
  problems: ImportProblem[]
}

export interface Course {
  id: number
  tournament_id: number
  name: string
  heat_interval_seconds: number
  delay_offset_seconds: number
}

export interface Group {
  id: number
  team_ids: string[]
}

export interface DivisionInfo {
  id: number
  name: string
  team_ids: string[]
}

export interface Round {
  id: number
  round_number: number
  status: string
  groups: Group[]
  divisions: DivisionInfo[]
}

export interface HeatResult {
  heat_id: number
  team_id: string
  time_seconds: number | null
  status: string
}

export interface Heat {
  id: number
  round_id: number
  group_id: number | null
  division_id: number | null
  course_id: number
  planned_start: string
  effective_start: string
  status: string
  results: HeatResult[]
}

export interface RankedTeam {
  rank: number
  team_id: string
  time_seconds: number | null
  status: string
}

export interface StandingsEntry {
  group_id: number | null
  division_id: number | null
  division_name: string | null
  ranked_teams: RankedTeam[]
}

export interface StandingsRound {
  id: number
  round_number: number
  status: string
  standings: StandingsEntry[]
}

export interface StandingsResponse {
  rounds: StandingsRound[]
}
