// Copyright 2025 Chaos Mesh Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package moa

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	config "github.com/chaos-mesh/chaos-mesh/pkg/config"
)

type fakeValidator struct {
	valid bool
	uid   string
}

func (f *fakeValidator) Validate(token string) (string, error) {
	if !f.valid {
		return "", errors.New("invalid token")
	}
	return f.uid, nil
}

func newTestService() *Service {
	conf := &config.ChaosDashboardConfig{
		MoaSecurityMode: true,
		MoaTokenHeader:  "Authorization",
	}
	return NewService(conf, logr.Discard())
}

func newTestRouter(s *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(s.Middleware)
	r.GET("/api/protected", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/api/common/config", func(c *gin.Context) {
		c.String(http.StatusOK, "config")
	})
	return r
}

func TestMiddleware_ValidTokenHeader(t *testing.T) {
	s := newTestService()
	s.SetValidator(&fakeValidator{valid: true, uid: "1001374"})
	r := newTestRouter(s)

	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMiddleware_MissingToken(t *testing.T) {
	s := newTestService()
	s.SetValidator(&fakeValidator{valid: false})
	r := newTestRouter(s)

	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMiddleware_InvalidToken(t *testing.T) {
	s := newTestService()
	s.SetValidator(&fakeValidator{valid: false})
	r := newTestRouter(s)

	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMiddleware_PublicPath(t *testing.T) {
	s := newTestService()
	s.SetValidator(&fakeValidator{valid: false})
	r := newTestRouter(s)

	req := httptest.NewRequest(http.MethodGet, "/api/common/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestExtractBearer(t *testing.T) {
	assert.Equal(t, "token", extractBearer("Bearer token"))
	assert.Equal(t, "token", extractBearer("token"))
	assert.Equal(t, "", extractBearer(""))
}
