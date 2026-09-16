package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/api/handler"
	"github.com/dreamreflex/service-edge/internal/api/middleware"
	"github.com/dreamreflex/service-edge/internal/config"
	"github.com/dreamreflex/service-edge/internal/service"
	"github.com/dreamreflex/service-edge/internal/store"
)

func TestFRPDistRouteIsBinaryAndMissingAPIIsNotSPA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	name := "frp_0.71.0_linux_amd64.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("archive-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.AgentAPIToken = "agent-token"
	svc := service.New(st, nil, cfg)
	r := NewRouter(Options{Cfg: cfg, Handler: &handler.Handler{Svc: svc}, JWT: middleware.NewJWTManager("secret", 0), FRPDistDir: dir})
	for _, tc := range []struct {
		path, want string
		status     int
	}{
		{"/api/v1/frp-dist/" + name, "archive-bytes", 200},
		{"/api/v1/missing", "", 404},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", tc.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s status=%d want=%d body=%q", tc.path, w.Code, tc.status, w.Body.String())
		}
		if tc.want != "" && w.Body.String() != tc.want {
			t.Fatalf("%s body=%q", tc.path, w.Body.String())
		}
	}
}
