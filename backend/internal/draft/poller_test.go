package draft

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

func TestPollerRunsOneLoopAndReconfigures(t *testing.T) {
	var mu sync.Mutex
	requests := make(map[string]int)
	active := 0
	maxActive := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	db, err := database.Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	client := sleeper.NewClient()
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()
	poller := NewPoller(db, client)
	defer poller.Stop()

	poller.Configure(database.Settings{PollingEnabled: true, PollingInterval: 15})
	time.Sleep(30 * time.Millisecond)
	if got := requestCount(&mu, requests, "/draft//picks"); got != 0 {
		t.Fatalf("requests without a configured draft = %d, want 0", got)
	}

	settings, err := database.UpdateSettings(context.Background(), db, database.EditableSettings{
		SleeperDraftID: "draft-a", PollingEnabled: true, PollingInterval: 15,
	})
	if err != nil {
		t.Fatalf("save draft-a settings: %v", err)
	}
	poller.Configure(settings)
	waitFor(t, func() bool { return requestCount(&mu, requests, "/draft/draft-a/picks") >= 2 })
	poller.Configure(settings)
	time.Sleep(40 * time.Millisecond)
	mu.Lock()
	observedMax := maxActive
	mu.Unlock()
	if observedMax != 1 {
		t.Fatalf("maximum concurrent poll requests = %d, want 1", observedMax)
	}

	settings, err = database.UpdateSettings(context.Background(), db, database.EditableSettings{
		SleeperDraftID: "draft-b", PollingEnabled: true, PollingInterval: 30,
	})
	if err != nil {
		t.Fatalf("save draft-b settings: %v", err)
	}
	poller.Configure(settings)
	waitFor(t, func() bool { return requestCount(&mu, requests, "/draft/draft-b/picks") >= 1 })
	waitFor(t, func() bool {
		state, stateErr := database.GetDraftState(context.Background(), db)
		return stateErr == nil && state.DraftID == "draft-b" && state.LastSyncedAt != nil
	})

	settings, err = database.UpdateSettings(context.Background(), db, database.EditableSettings{
		SleeperDraftID: "draft-b", PollingEnabled: false, PollingInterval: 30,
	})
	if err != nil {
		t.Fatalf("disable polling settings: %v", err)
	}
	poller.Configure(settings)
	mu.Lock()
	stoppedCount := requests["/draft/draft-b/picks"]
	mu.Unlock()
	time.Sleep(60 * time.Millisecond)
	if got := requestCount(&mu, requests, "/draft/draft-b/picks"); got != stoppedCount {
		t.Fatalf("requests after disable = %d, want unchanged %d", got, stoppedCount)
	}

	state, err := database.GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() error = %v", err)
	}
	if state.DraftID != "draft-b" || state.PollingEnabled || state.LastSyncedAt == nil {
		t.Fatalf("stopped draft state = %#v", state)
	}
}

func requestCount(mu *sync.Mutex, requests map[string]int, path string) int {
	mu.Lock()
	defer mu.Unlock()
	return requests[path]
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
