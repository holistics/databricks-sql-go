package dbsql

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	dbsqlerr "github.com/databricks/databricks-sql-go/errors"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestHTTPStatusError(t *testing.T) {
	// Starts a thrift test server that answers every request with failStatus once it is non-zero.
	newServer := func(t *testing.T, reasonPhrase string) (*sql.DB, *atomic.Int32) {
		state := &callState{}
		loadTestData(t, "OpenSessionSuccess.json", &state.openSessionResp)
		loadTestData(t, "CloseSessionSuccess.json", &state.closeSessionResp)

		var failStatus atomic.Int32
		thriftServer := getServer(state)
		t.Cleanup(thriftServer.Close)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if code := int(failStatus.Load()); code != 0 {
				if reasonPhrase != "" {
					w.Header().Set("X-Databricks-Reason-Phrase", reasonPhrase)
				}
				w.WriteHeader(code)
				return
			}
			thriftServer.Config.Handler.ServeHTTP(w, r)
		}))
		t.Cleanup(ts.Close)

		u, err := url.Parse(ts.URL)
		require.NoError(t, err)
		port, err := strconv.Atoi(u.Port())
		require.NoError(t, err)

		connector, err := NewConnector(
			WithServerHostname("localhost"),
			WithPort(port),
			WithHTTPPath(""),
			WithAccessToken(""),
			WithRetries(-1, 0, 0),
		)
		require.NoError(t, err)
		db := sql.OpenDB(connector)
		t.Cleanup(func() { db.Close() })

		return db, &failStatus
	}

	requireHTTPStatus := func(t *testing.T, err error, code int, status string) {
		require.Error(t, err)
		var httpErr *dbsqlerr.HTTPStatusError
		require.True(t, errors.As(err, &httpErr), "no HTTPStatusError in chain: %v", err)
		require.Equal(t, code, httpErr.StatusCode)
		require.Equal(t, status, httpErr.Status)
		require.Contains(t, err.Error(), "unexpected HTTP status "+status)
	}

	for _, tc := range []struct {
		code         int
		status       string
		reasonPhrase string
	}{
		{http.StatusUnauthorized, "401 Unauthorized", ""},
		{http.StatusForbidden, "403 Forbidden", ""},
		{http.StatusUnauthorized, "401 Unauthorized", "invalid token"},
		{http.StatusForbidden, "403 Forbidden", "permission denied"},
	} {
		t.Run(tc.status+" on connect, reason phrase "+strconv.Quote(tc.reasonPhrase), func(t *testing.T) {
			db, failStatus := newServer(t, tc.reasonPhrase)
			failStatus.Store(int32(tc.code))

			_, err := db.Conn(context.Background())
			requireHTTPStatus(t, err, tc.code, tc.status)
			require.True(t, errors.Is(err, dbsqlerr.RequestError))
			if tc.reasonPhrase != "" {
				require.Contains(t, err.Error(), tc.reasonPhrase)
			}
		})

		t.Run(tc.status+" on query, reason phrase "+strconv.Quote(tc.reasonPhrase), func(t *testing.T) {
			db, failStatus := newServer(t, tc.reasonPhrase)
			conn, err := db.Conn(context.Background())
			require.NoError(t, err)
			defer conn.Close()
			failStatus.Store(int32(tc.code))

			_, err = conn.QueryContext(context.Background(), "select 1")
			requireHTTPStatus(t, err, tc.code, tc.status)
			require.True(t, errors.Is(err, dbsqlerr.ExecutionError))
		})
	}
}
