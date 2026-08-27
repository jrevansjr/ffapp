// Package fantasypros retrieves the two approved 2026 draft datasets. It owns
// the provider wire format but has no database responsibilities.
package fantasypros

import "time"

// Season is the draft season populated by milestone M6.3.
const Season = 2026

// DatasetName identifies one independently refreshed FantasyPros response.
type DatasetName string

const (
	DatasetADP DatasetName = "adp"
	DatasetECR DatasetName = "ecr"
)

// ADPRanking is one player's FantasyPros Aggregate ADP.
type ADPRanking struct {
	FantasyProsID string
	Name          string
	Position      string
	Team          string
	ADP           float64
}

// ADPDataset is the validated 2026 half-PPR Aggregate ADP response.
type ADPDataset struct {
	UpdatedAt time.Time
	Rankings  []ADPRanking
}

// ExpertRanking is one player's overall draft ECR, position rank, tier, and
// the range of ranks submitted by the included experts.
type ExpertRanking struct {
	FantasyProsID string
	Name          string
	Position      string
	Team          string
	OverallRank   int
	PositionRank  int
	Tier          int
	RankMin       int
	RankMax       int
	RankStdDev    float64
}

// ECRDataset is the validated 2026 half-PPR Draft ECR response.
type ECRDataset struct {
	UpdatedAt time.Time
	Rankings  []ExpertRanking
}

// DatasetNames returns the two caches required for an offline database build.
func DatasetNames() []DatasetName {
	return []DatasetName{DatasetADP, DatasetECR}
}
