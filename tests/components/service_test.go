//go:build integration

package components_test

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	fgamodel "auth-service/fga"
	"auth-service/internal/config"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type fixture struct {
	store                  store
	db                     *sql.DB
	owner, telegram, space string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	cfg, err := config.LoadConfig("/components.yaml")
	require.NoError(t, err)
	pg := cfg.Postgres
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", pg.Host, pg.Port, pg.User, pg.Password, pg.DBName))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	f := &fixture{store: newStore(t, fgamodel.Strict), db: db, space: uuid.NewString()}
	f.owner, f.telegram = f.actor(t)
	f.store.write(t, []key{{"user:" + f.owner, "owner", "space:" + f.space}, {"user:*", "shared", "space:" + f.space}, {"user:*", "created", "space:" + f.space}})
	return f
}

func (f *fixture) actor(t *testing.T) (string, string) {
	t.Helper()
	id := uuid.New()
	tg := int64(binary.BigEndian.Uint32(id[:4])) + 1
	_, err := f.db.ExecContext(t.Context(), "INSERT INTO users.users(id) VALUES($1)", id)
	require.NoError(t, err)
	_, err = f.db.ExecContext(t.Context(), "INSERT INTO users.telegram(id,tg_id,user_id,global_invite_policy) VALUES($1,$2,$3,'ALLOW_ALL')", uuid.New(), tg, id)
	require.NoError(t, err)
	return id.String(), strconv.FormatInt(tg, 10)
}

// Лишь один процесс в выделенном network namespace занимает этот тестовый порт.
// Основной штатный контейнер продолжает слушать 8080. Тесты процессов последовательны.
const baseURL = "http://127.0.0.1:18181"

type process struct {
	cmd     *exec.Cmd
	done    chan struct{}
	err     error
	logfile string
}

func (f *fixture) launch(t *testing.T, path, api string) *process {
	t.Helper()
	cfg, err := config.LoadConfig("/components.yaml")
	require.NoError(t, err)
	cfg.Server.Port = 18181
	cfg.OpenFGA.APIURL = api
	cfg.OpenFGA.StoreID = f.store.id
	cfg.OpenFGA.ModelID = f.store.model
	cfg.OpenFGA.ApplyModelOnStart = false
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	dir := t.TempDir()
	name := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(name, data, 0600))
	log, err := os.CreateTemp(dir, "service-*.log")
	require.NoError(t, err)
	p := &process{cmd: exec.Command(path, "-config", name), done: make(chan struct{}), logfile: log.Name()}
	p.cmd.Dir = "/app"
	p.cmd.Stdout = log
	p.cmd.Stderr = log
	require.NoError(t, p.cmd.Start())
	require.NoError(t, log.Close())
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	t.Cleanup(func() { p.stop(t) })
	return p
}

func (p *process) stop(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
		t.Error("service did not stop in time")
	}
}
func healthy() bool {
	client := http.Client{Timeout: 200 * time.Millisecond}
	res, err := client.Get(baseURL + "/api/v0/health")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == 200
}
func (p *process) ready(t *testing.T) {
	t.Helper()
	deadline := time.After(20 * time.Second)
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			t.Fatalf("auth-service exited before readiness: %v", p.err)
		case <-deadline:
			t.Fatal("auth-service readiness deadline")
		case <-ticker.C:
			if healthy() {
				return
			}
		}
	}
}

func login(t *testing.T) string {
	t.Helper()
	cfg, err := config.LoadConfig("/components.yaml")
	require.NoError(t, err)
	secret, _ := request(t, "GET", cfg.Vault.Address+"/v1/"+cfg.Vault.SecretsPath+"/bot", nil,
		map[string]string{"X-Vault-Token": cfg.Vault.Token}, http.StatusOK)
	key := secret["data"].(object)["data"].(object)["api_key"]
	res, _ := request(t, "POST", baseURL+"/api/v0/auth/login", object{
		"grant_type": "client_credentials", "client_id": "bot", "client_secret": key, "scope": "bot",
	}, nil, 200)
	token, ok := res["access_token"].(string)
	require.True(t, ok && token != "")
	return token
}

func (f *fixture) body(action, kind, id string, resource, attrs object) object {
	return object{"request_id": uuid.NewString(), "subject": object{"type": "user", "id": f.owner},
		"action": object{"name": action, "attributes": attrs}, "resource": object{"type": kind, "id": id, "attributes": resource},
		"options": object{"include_reasons": true},
		"context": object{"client": "untrusted", "service": "untrusted", "ip": "192.0.2.1", "time": "2001-01-01T00:00:00Z"}}
}
func (f *fixture) check(t *testing.T, token string, body object, status int) object {
	t.Helper()
	out, headers := request(t, "POST", baseURL+"/api/v0/decisions/check", body, map[string]string{
		"Authorization": "Bearer " + token, "X-Telegram-User-Id": f.telegram,
	}, status)
	require.Equal(t, body["request_id"], out["request_id"])
	require.Equal(t, body["request_id"], headers.Get("X-Request-ID"))
	if status == 200 {
		require.Contains(t, []string{"allow", "deny"}, out["result"])
		require.Equal(t, out["result"] == "allow", out["allow"])
		require.Equal(t, f.store.model, out["meta"].(object)["authorization_model_id"])
	}
	return out
}
