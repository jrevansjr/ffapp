// Package fantasypros retrieves the approved 2026 draft datasets. It owns
// the provider wire format but has no database responsibilities.
package fantasypros

import "time"

// Season is the draft season shared by rankings and preseason projections.
const Season = 2026

// DatasetName identifies one independently refreshed FantasyPros response.
type DatasetName string

const (
	DatasetADP         DatasetName = "adp"
	DatasetECR         DatasetName = "ecr"
	DatasetProjections DatasetName = "projections"
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

// PlayerProjection is one player's FantasyPros preseason volume forecast.
// Nil fields distinguish a statistic that FantasyPros does not project for the
// player's position from a real projection of zero.
type PlayerProjection struct {
	FantasyProsID       string
	Name                string
	Position            string
	Team                string
	PassingYards        *float64
	PassingTouchdowns   *float64
	RushingYards        *float64
	RushingTouchdowns   *float64
	ReceivingYards      *float64
	ReceivingTouchdowns *float64
}

// ProjectionDataset is the validated 2026 preseason QB/RB/WR/TE response.
type ProjectionDataset struct {
	UpdatedAt   time.Time
	Projections []PlayerProjection
}

// DatasetNames returns every cache required for an offline database build.
func DatasetNames() []DatasetName {
	return []DatasetName{DatasetADP, DatasetECR, DatasetProjections}
}
