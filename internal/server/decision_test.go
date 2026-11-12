package server

import (
	"auth-service/internal/server/mocks"
	"github.com/golang/mock/gomock"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDecisionRoute(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	handler := mocks.NewMockhandler(ctrl)
	middleware := mocks.NewMockmiddlewareHandler(ctrl)

	handler.EXPECT().Version().Return("v0")
	middleware.EXPECT().CheckToken(gomock.Any()).DoAndReturn(func(next echo.HandlerFunc) echo.HandlerFunc { return next }).AnyTimes()
	middleware.EXPECT().CheckDecisionToken(gomock.Any()).DoAndReturn(func(next echo.HandlerFunc) echo.HandlerFunc { return next }).AnyTimes()
	s, err := New(WithPort(8080), WithHandlerV0(handler), WithMiddlewareHandler(middleware), WithShutdownTimeout(time.Second))
	require.NoError(t, err)
	require.NoError(t, s.createRoutes())

	routes := map[string]bool{}
	for _, route := range s.e.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	require.True(t, routes["POST /v0/decisions/check"])
	require.False(t, routes["POST /api/v0/resources/decisions/check"])
	require.True(t, routes["POST /api/v0/resources/update"])
	require.True(t, routes["POST /api/v0/auth/login"])
}
