// Package analyzer provides core analysis utilities for Repo-lyzer.
// This file defines common types such as Metric, Thresholds, and a WeightedScoreEngine
// used by various risk analysis components.

package analyzer

// Metric represents a single analysis metric with a score and weighting.
// Score is expected to be in the range 0-100 (higher is worse risk).
// Weight determines the importance of the metric in the aggregated score.
// Description provides a human‑readable explanation of the metric.
type Metric struct {
    Name        string  `json:"name"`
    Score       float64 `json:"score"`
    Weight      float64 `json:"weight"`
    Description string  `json:"description,omitempty"`
}

// Thresholds define the score boundaries for health categories.
// The values represent the score cut‑offs for warning, healthy and excellent levels.
// These are used by the WeightedScoreEngine to interpret the final aggregated score.
type Thresholds struct {
    Warning   float64 `json:"warning"`
    Healthy   float64 `json:"healthy"`
    Excellent float64 `json:"excellent"`
}

// WeightedScoreEngine aggregates a set of Metrics into a single score.
// It applies the provided thresholds to interpret the overall health.
// MaxScore caps the final computed score (commonly 100).
type WeightedScoreEngine struct {
    MaxScore   float64
    Thresholds Thresholds
}

// NewWeightedScoreEngine creates a new engine with a maximum score and thresholds.
func NewWeightedScoreEngine(maxScore float64, thresholds Thresholds) *WeightedScoreEngine {
    return &WeightedScoreEngine{MaxScore: maxScore, Thresholds: thresholds}
}

// CalculateScore computes the weighted average score of the supplied metrics.
// It returns the aggregated score capped at MaxScore. If no metrics are provided,
// the function returns 0 without error.
func (w *WeightedScoreEngine) CalculateScore(metrics []Metric) (float64, error) {
    if len(metrics) == 0 {
        return 0, nil
    }
    var totalWeight, weightedSum float64
    for _, m := range metrics {
        totalWeight += m.Weight
        weightedSum += m.Score * m.Weight
    }
    if totalWeight == 0 {
        return 0, nil
    }
    score := weightedSum / totalWeight
    if score > w.MaxScore {
        score = w.MaxScore
    }
    return score, nil
}
