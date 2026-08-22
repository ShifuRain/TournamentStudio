CREATE UNIQUE INDEX idx_pre_phase_rounds_tournament_round
    ON pre_phase_rounds (tournament_id, round_number);
