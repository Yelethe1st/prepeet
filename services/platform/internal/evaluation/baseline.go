package evaluation

// Personal delivery baselines: ART-07.
//
// A candidate is compared with their own history, never with a universal
// standard. A baseline exists only after MinBaselineSessions measured
// practice sessions, and the range it draws is the middle half of those
// sessions' own values: guidance about where this person usually sits,
// deliberately not a target. Screening is unreachable by construction:
// the history is read under the candidate's practice scope, and a
// screening analysis lives under a tenant that scope cannot see.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/evaluation/db"
)

// MinBaselineSessions is the documented minimum before a range is drawn.
// Fewer sessions describe the sessions, not the person.
const MinBaselineSessions = 5

// BaselineVersion names the derivation.
const BaselineVersion = "baseline-1"

// BaselineNote ships with every baseline: a range is where this person
// usually sits, never a correct rate.
const BaselineNote = "These ranges are where your own measured sessions usually sit. " +
	"They are guidance about you, not a target: there is no correct speaking rate."

// Range is the middle half of a metric's history.
type Range struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// Baseline is one candidate's personal ranges, or the honest absence.
type Baseline struct {
	BaselineVersion  string           `json:"baseline_version"`
	SessionsMeasured int              `json:"sessions_measured"`
	MinimumSessions  int              `json:"minimum_sessions"`
	Ready            bool             `json:"ready"`
	Ranges           map[string]Range `json:"ranges"`
	Note             string           `json:"note"`
}

// ArticulationHistory answers every delivery analysis the scope can see
// for the given mode, oldest first.
func (s *Store) ArticulationHistory(ctx context.Context, ref SessionRef) ([]Articulation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluation: beginning history: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ListArticulation(ctx, ref.Mode)
	if err != nil {
		return nil, fmt.Errorf("evaluation: listing articulation: %w", err)
	}
	history := make([]Articulation, 0, len(rows))
	for _, row := range rows {
		var warnings []string
		_ = json.Unmarshal(row.Warnings, &warnings)
		history = append(history, Articulation{
			ID: row.ID, SessionID: row.SessionID, Status: row.Status, Warnings: warnings,
			Document: json.RawMessage(row.Analysis), CalculationVersion: row.CalculationVersion,
			PolicyVersion: row.PolicyVersion, InputDigest: row.InputDigest, CreatedAt: row.CreatedAt,
		})
	}
	return history, nil
}

// baselineMetrics are the analysis metrics a range is drawn for.
var baselineMetrics = []string{"words_per_minute", "fillers_per_100_words", "long_pause_count"}

// DeriveBaseline draws the ranges from assessable analyses, or answers
// the honest absence with the count and the minimum.
func DeriveBaseline(history []Articulation) Baseline {
	values := map[string][]float64{}
	measured := 0
	for _, analysis := range history {
		if analysis.Status != "assessable" && analysis.Status != "partially_assessable" {
			continue
		}
		var document struct {
			Metrics map[string]float64 `json:"metrics"`
		}
		if err := json.Unmarshal(analysis.Document, &document); err != nil || document.Metrics == nil {
			continue
		}
		measured++
		for _, metric := range baselineMetrics {
			if value, present := document.Metrics[metric]; present {
				values[metric] = append(values[metric], value)
			}
		}
	}

	baseline := Baseline{
		BaselineVersion: BaselineVersion, SessionsMeasured: measured,
		MinimumSessions: MinBaselineSessions, Ranges: map[string]Range{}, Note: BaselineNote,
	}
	if measured < MinBaselineSessions {
		return baseline
	}
	baseline.Ready = true
	for _, metric := range baselineMetrics {
		series := values[metric]
		if len(series) < MinBaselineSessions {
			continue
		}
		sort.Float64s(series)
		baseline.Ranges[metric] = Range{
			Low:  quantile(series, 0.25),
			High: quantile(series, 0.75),
		}
	}
	return baseline
}

// quantile is the nearest-rank value at q of a sorted series: no
// interpolation, so a range is always made of values that occurred.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(q*float64(len(sorted)) + 0.5)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}
