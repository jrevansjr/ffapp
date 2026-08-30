// Package draft owns the one background loop that synchronizes Sleeper picks
// into SQLite. API requests only read the resulting local state.
package draft

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

type pollerConfig struct {
	draftID  string
	leagueID string
	interval time.Duration
}

// Poller manages at most one live synchronization goroutine. Configure safely
// stops the prior loop before starting a replacement when Admin settings change.
type Poller struct {
	db     *sql.DB
	client *sleeper.Client

	mu     sync.Mutex
	config pollerConfig
	cancel context.CancelFunc
	done   chan struct{}
}

// NewPoller creates an idle poller using the server's shared database handle.
func NewPoller(db *sql.DB, client *sleeper.Client) *Poller {
	return &Poller{db: db, client: client}
}

// Configure applies persisted settings. Disabled or unconfigured settings stop
// the loop; changed draft IDs or intervals replace it and sync immediately.
func (poller *Poller) Configure(settings database.Settings) {
	poller.mu.Lock()
	defer poller.mu.Unlock()

	next := pollerConfig{
		draftID:  settings.SleeperDraftID,
		leagueID: settings.SleeperLeagueID,
		interval: time.Duration(settings.PollingInterval) * time.Millisecond,
	}
	shouldRun := settings.PollingEnabled && next.draftID != ""
	if shouldRun && poller.cancel != nil && poller.config == next {
		return
	}
	if poller.cancel != nil {
		poller.cancel()
		<-poller.done
		poller.cancel = nil
		poller.done = nil
	}
	poller.config = pollerConfig{}
	if !shouldRun {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	poller.config = next
	poller.cancel = cancel
	poller.done = done
	go poller.run(ctx, done, next)
}

// Stop cancels the active request/ticker and waits for the worker to exit.
func (poller *Poller) Stop() {
	poller.mu.Lock()
	defer poller.mu.Unlock()
	if poller.cancel == nil {
		return
	}
	poller.cancel()
	<-poller.done
	poller.cancel = nil
	poller.done = nil
	poller.config = pollerConfig{}
}

func (poller *Poller) run(ctx context.Context, done chan struct{}, config pollerConfig) {
	defer close(done)
	lastPickCount := -1
	lastUnknownCount := -1
	pickCount, unknownCount, succeeded := poller.sync(ctx, config)
	if succeeded {
		logSyncChange(config.draftID, pickCount, unknownCount, &lastPickCount, &lastUnknownCount)
	}
	if ctx.Err() != nil {
		return
	}
	ticker := time.NewTicker(config.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pickCount, unknownCount, succeeded := poller.sync(ctx, config)
			if succeeded {
				logSyncChange(config.draftID, pickCount, unknownCount, &lastPickCount, &lastUnknownCount)
			}
		}
	}
}

func (poller *Poller) sync(ctx context.Context, config pollerConfig) (int, int, bool) {
	picks, err := poller.client.FetchDraftPicks(ctx, config.draftID)
	if err != nil {
		if ctx.Err() != nil {
			return 0, 0, false
		}
		log.Printf("Sleeper draft %s sync failed: %v", config.draftID, err)
		if recordErr := database.RecordDraftSyncFailure(
			ctx, poller.db, config.draftID, config.leagueID,
		); recordErr != nil {
			log.Printf("record Sleeper draft %s failure: %v", config.draftID, recordErr)
		}
		return 0, 0, false
	}

	inputs := make([]database.DraftPickInput, 0, len(picks))
	for _, pick := range picks {
		inputs = append(inputs, database.DraftPickInput{
			PickNumber:      pick.PickNumber,
			Round:           pick.Round,
			DraftSlot:       pick.DraftSlot,
			RosterID:        pick.RosterID.Value,
			PickedBy:        pick.PickedBy.Value,
			SleeperPlayerID: pick.SleeperPlayerID.Value,
			FirstName:       pick.Metadata.FirstName.Value,
			LastName:        pick.Metadata.LastName.Value,
			Position:        pick.Metadata.Position.Value,
			Team:            pick.Metadata.Team.Value,
		})
	}
	unknown, err := database.SyncDraftPicks(
		ctx, poller.db, config.draftID, config.leagueID, inputs,
	)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("persist Sleeper draft %s picks: %v", config.draftID, err)
		}
		return 0, 0, false
	}
	return len(inputs), unknown, true
}

func logSyncChange(draftID string, picks, unknown int, lastPicks, lastUnknown *int) {
	if picks == *lastPicks && unknown == *lastUnknown {
		return
	}
	log.Printf(
		"Sleeper draft %s synchronized: %d picks, %d unknown player IDs",
		draftID, picks, unknown,
	)
	*lastPicks = picks
	*lastUnknown = unknown
}
