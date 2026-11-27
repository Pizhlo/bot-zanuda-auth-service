//go:build integration

package components_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/stretchr/testify/require"
)

type object = map[string]any

const fgaURL = "http://open-fga:8081"

func request(t *testing.T, method, endpoint string, body any, headers map[string]string, status int) (object, http.Header) {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), method, endpoint, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	data, err = io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, status, res.StatusCode, "unexpected HTTP status for %s", req.URL.Path)
	out := object{}
	if len(data) > 0 {
		require.NoError(t, json.Unmarshal(data, &out))
	}
	return out, res.Header
}

type store struct{ id, model string }

func newStore(t *testing.T, dsl string) store {
	t.Helper()
	// Проект testkit уничтожается целиком даже при утрате ответа CreateStore.
	res, _ := request(t, "POST", fgaURL+"/stores", object{"name": "components-" + uuid.NewString()}, nil, 201)
	s := store{id: res["id"].(string)}
	model, err := transformer.TransformDSLToJSON(dsl)
	require.NoError(t, err)
	res, _ = request(t, "POST", s.url()+"/authorization-models", json.RawMessage(model), nil, 201)
	s.model = res["authorization_model_id"].(string)
	return s
}
func (s store) url() string { return fgaURL + "/stores/" + s.id }

type key struct {
	User     string `json:"user"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
}

func (s store) write(t *testing.T, keys []key) {
	t.Helper()
	for batch := range slices.Chunk(keys, 100) {
		request(t, "POST", s.url()+"/write", object{"authorization_model_id": s.model, "writes": object{"tuple_keys": batch}}, nil, 200)
	}
}
func (s store) graph(t *testing.T) map[key]bool {
	t.Helper()
	graph := map[key]bool{}
	token := ""
	for {
		res, _ := request(t, "POST", s.url()+"/read", object{"continuation_token": token, "consistency": "HIGHER_CONSISTENCY"}, nil, 200)
		for _, row := range res["tuples"].([]any) {
			fields := row.(object)["key"].(object)
			graph[key{fields["user"].(string), fields["relation"].(string), fields["object"].(string)}] = true
		}
		token, _ = res["continuation_token"].(string)
		if token == "" {
			return graph
		}
	}
}

type call struct {
	path string
	body object
}
type traffic struct {
	url   string
	mu    sync.Mutex
	calls []call
}

// Записываем запрос до ответа OpenFGA, в том числе неуспешную попытку write.
func observe(t *testing.T) *traffic {
	t.Helper()
	upstream, err := url.Parse(fgaURL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	capture := &traffic{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = r.Body.Close()
		var body object
		if len(data) > 0 {
			if err := json.Unmarshal(data, &body); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		capture.mu.Lock()
		capture.calls = append(capture.calls, call{r.URL.Path, body})
		capture.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(data))
		proxy.ServeHTTP(w, r)
	}))
	capture.url = server.URL
	t.Cleanup(server.Close)
	return capture
}
func (c *traffic) take(endpoint string) []object {
	c.mu.Lock()
	defer c.mu.Unlock()
	var selected []object
	for _, item := range c.calls {
		if strings.HasSuffix(item.path, endpoint) {
			selected = append(selected, item.body)
		}
	}
	return selected
}
func (c *traffic) reset() { c.mu.Lock(); defer c.mu.Unlock(); c.calls = nil }
