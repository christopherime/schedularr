package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/store"
)

// newCronTestServer is newTestServer with an explicit Deps.Location, which
// is the one dependency GET /cron/next actually reads. A store is still
// wired because NewRouter's other handlers share these Deps -- this
// endpoint itself touches nothing.
func newCronTestServer(t *testing.T, loc *time.Location) http.Handler {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })

	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test", Location: loc})
	return gen.HandlerFromMux(h, chi.NewRouter())
}

func decodeOccurrences(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var body struct {
		Occurrences []string `json:"occurrences"`
	}
	require.NoError(t, json.NewDecoder(strings.NewReader(w.Body.String())).Decode(&body), "body: %s", w.Body.String())
	return body.Occurrences
}

func TestNextCronOccurrences_ReturnsThreeByDefault(t *testing.T) {
	t.Parallel()
	h := newCronTestServer(t, time.UTC)

	w := doRequest(t, h, http.MethodGet, "/cron/next?expr=0+6+*+*+*&from=2026-09-10T12:00:00Z", nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	got := decodeOccurrences(t, w)
	require.Len(t, got, 3)
	require.Equal(t, "2026-09-11T06:00:00Z", got[0])
}

func TestNextCronOccurrences_RejectsAnUnparseableExpression(t *testing.T) {
	t.Parallel()
	h := newCronTestServer(t, time.UTC)

	w := doRequest(t, h, http.MethodGet, "/cron/next?expr=nonsense", nil)
	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}

func TestNextCronOccurrences_RejectsCountOutOfRange(t *testing.T) {
	t.Parallel()
	h := newCronTestServer(t, time.UTC)

	// The contract declares minimum 1 / maximum 10, but oapi-codegen does
	// not enforce either, so these only 400 because the handler checks.
	for _, count := range []string{"0", "11", "-1"} {
		w := doRequest(t, h, http.MethodGet, "/cron/next?expr=0+6+*+*+*&count="+count, nil)
		require.Equal(t, http.StatusBadRequest, w.Code, "count=%s body: %s", count, w.Body.String())
	}
}

func TestNextCronOccurrences_EmptyForAScheduleThatNeverFires(t *testing.T) {
	t.Parallel()
	h := newCronTestServer(t, time.UTC)

	w := doRequest(t, h, http.MethodGet, "/cron/next?expr=0+0+30+2+*", nil)
	require.Equal(t, http.StatusOK, w.Code,
		"the expression is valid, it just never fires -- body: %s", w.Body.String())
	require.Empty(t, decodeOccurrences(t, w))
	require.NotContains(t, w.Body.String(), "0001-01-01", "a zero time serialized into the response")
}

func TestNextCronOccurrences_EvaluatesInTheConfiguredLocation(t *testing.T) {
	t.Parallel()
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	h := newCronTestServer(t, zurich)

	w := doRequest(t, h, http.MethodGet, "/cron/next?expr=0+6+*+*+*&from=2026-09-10T12:00:00Z&count=1", nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	got := decodeOccurrences(t, w)
	require.Len(t, got, 1)
	at, err := time.Parse(time.RFC3339, got[0])
	require.NoError(t, err)
	// 06:00 Zurich in September is 04:00Z: the handler must evaluate the
	// wall-clock field in the configured zone, not in UTC.
	require.Equal(t, 4, at.UTC().Hour(), "occurrence %s is not 06:00 Zurich", got[0])
}

func TestNextCronOccurrences_NilLocationFallsBackToLocal(t *testing.T) {
	t.Parallel()
	// Deps.Location unset is a real configuration -- every test server in
	// this package leaves it nil -- so it must answer rather than panic.
	h := newCronTestServer(t, nil)

	w := doRequest(t, h, http.MethodGet, "/cron/next?expr=@daily&count=1", nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.Len(t, decodeOccurrences(t, w), 1)
}
