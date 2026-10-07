package routers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetMetrics(t *testing.T) {
	const payload = `# TYPE requests_total counter
requests_total{method="get"} 5
requests_total{method="post"} 7
# TYPE latency histogram
latency_bucket{le="1"} 2
latency_bucket{le="+Inf"} 3
latency_sum 4
latency_count 3
# TYPE keep gauge
keep 9
`
	tests := []struct {
		name   string
		query  string
		status int
		reset  bool
	}{
		{"comma separated", "?metrics=requests_total,latency", http.StatusNoContent, true},
		{"repeated parameter", "?metrics=requests_total&metrics=latency", http.StatusNoContent, true},
		{"duplicates and whitespace", "?metrics=%20requests_total%20,latency,requests_total", http.StatusNoContent, true},
		{"unknown alongside known", "?metrics=requests_total,missing,latency", http.StatusNoContent, true},
		{"unknown only", "?metrics=missing", http.StatusNoContent, false},
		{"missing parameter", "", http.StatusBadRequest, false},
		{"empty parameter", "?metrics=", http.StatusBadRequest, false},
		{"blank parameter", "?metrics=%20", http.StatusBadRequest, false},
		{"empty list item", "?metrics=requests_total,", http.StatusBadRequest, false},
		{"empty repeated parameter", "?metrics=requests_total&metrics=", http.StatusBadRequest, false},
		{"invalid name", "?metrics=requests_total,bad-name", http.StatusBadRequest, false},
		{"wildcard", "?metrics=*", http.StatusBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter(ApiRouterConfig{CorsDomain: "*"})
			request := func(method, path, body string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
				return w
			}
			require.Equal(t, http.StatusAccepted, request(http.MethodPost, "/metrics", payload).Code)
			before := request(http.MethodGet, "/metrics", "").Body.String()

			// Repeating a successful reset is harmless; failures never partially reset.
			for i := 0; i < 2; i++ {
				w := request(http.MethodGet, "/reset"+tt.query, "")
				require.Equal(t, tt.status, w.Code, w.Body.String())
				assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				after := request(http.MethodGet, "/metrics", "").Body.String()
				if tt.reset {
					assert.Equal(t, "# TYPE keep gauge\nkeep 9\n", after)
				} else {
					assert.Equal(t, before, after)
				}
			}

			if tt.reset {
				require.Equal(t, http.StatusAccepted, request(http.MethodPost, "/metrics", payload).Code)
				assert.Equal(t, before, request(http.MethodGet, "/metrics", "").Body.String())
			}
		})
	}
}

func TestResetMetricsAccess(t *testing.T) {
	for _, tt := range []struct {
		name     string
		password string
		origin   string
		status   int
	}{
		{"missing credentials", "", "https://allowed", http.StatusUnauthorized},
		{"wrong credentials", "wrong", "https://allowed", http.StatusUnauthorized},
		{"disallowed origin", "password", "https://disallowed", http.StatusForbidden},
		{"authenticated", "password", "https://allowed", http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter(ApiRouterConfig{CorsDomain: "https://allowed", Accounts: []string{"user=password"}})
			insert := httptest.NewRequest(http.MethodPost, "/metrics", strings.NewReader("counter 5\n"))
			insert.SetBasicAuth("user", "password")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, insert)
			require.Equal(t, http.StatusAccepted, w.Code)

			req := httptest.NewRequest(http.MethodGet, "/reset?metrics=counter", nil)
			req.Header.Set("Origin", tt.origin)
			if tt.password != "" {
				req.SetBasicAuth("user", tt.password)
			}
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tt.status, w.Code)

			w = httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			if tt.status == http.StatusNoContent {
				assert.Empty(t, w.Body.String())
			} else {
				assert.Contains(t, w.Body.String(), "counter 5\n")
			}
		})
	}
}
