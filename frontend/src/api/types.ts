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
