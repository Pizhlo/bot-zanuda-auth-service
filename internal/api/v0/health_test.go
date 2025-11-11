package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	version := "1.0.0"
	buildDate := "2021-01-01"
	gitCommit := "1234567890"

	handler, err := New(
		WithVersion(version),
		WithBuildDate(buildDate),
		WithGitCommit(gitCommit),
	)
	require.NoError(t, err)

	r := runTestServer(t, handler)

	ts := httptest.NewServer(r)
	defer ts.Close()

	resp := testRequest(t, ts, http.MethodGet, "/api/v0/health", "", nil)

	defer func() {
		require.NoError(t, resp.Body.Close())
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	expectedBody := map[string]string{
		"version":   version,
		"buildDate": buildDate,
		"gitCommit": gitCommit,
	}

	assertResponse(t, resp, expectedBody)
}

func assertResponse(t *testing.T, resp *http.Response, body map[string]string) {
	t.Helper()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	actualBody := map[string]string{}

	err := json.NewDecoder(resp.Body).Decode(&actualBody)
	require.NoError(t, err)

	assert.Equal(t, body, actualBody)
}
