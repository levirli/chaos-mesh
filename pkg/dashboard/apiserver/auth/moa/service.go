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
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"

	config "github.com/chaos-mesh/chaos-mesh/pkg/config"
	"github.com/chaos-mesh/chaos-mesh/pkg/dashboard/apiserver/utils"
)

// TokenValidator validates a MOA token and returns the user identifier or an error.
// The concrete implementation should call the MOA framework permission validation routine.
type TokenValidator interface {
	Validate(token string) (string, error)
}

// Service provides MOA authentication middleware for the Chaos Dashboard.
type Service struct {
	conf      *config.ChaosDashboardConfig
	logger    logr.Logger
	validator TokenValidator
}

// NewService returns a new MOA authentication service.
func NewService(
	conf *config.ChaosDashboardConfig,
	logger logr.Logger,
) *Service {
	return &Service{
		conf:      conf,
		logger:    logger.WithName("moa-auth"),
		validator: NewRemoteValidator(conf.MoaProjectId, http.DefaultClient),
	}
}

// SetValidator replaces the default token validator. It is intended for tests.
func (s *Service) SetValidator(v TokenValidator) {
	s.validator = v
}

// Register mounts the MOA authentication middleware when MOA security mode is enabled.
// It protects all /api/* routes except the public whitelist. Static assets and the
// root page are left untouched because they are served outside the /api group.
func Register(r *gin.RouterGroup, s *Service, conf *config.ChaosDashboardConfig) {
	if !conf.MoaSecurityMode {
		return
	}

	r.Use(s.Middleware)
}

// Middleware checks the MOA token on protected routes.
func (s *Service) Middleware(c *gin.Context) {
	// Public endpoints that must be reachable before authentication.
	if isPublicPath(c.Request.URL.Path) {
		c.Next()
		return
	}

	token := s.extractToken(c)
	if token == "" {
		s.logger.V(1).Info("missing MOA token", "path", c.Request.URL.Path)
		utils.SetAPIError(c, utils.ErrUnauthorized.New("Unauthorized: missing MOA token"))
		c.Abort()
		return
	}

	uid, err := s.validator.Validate(token)
	if err != nil {
		s.logger.V(1).Info("invalid MOA token", "error", err.Error())
		utils.SetAPIError(c, utils.ErrUnauthorized.New("Unauthorized: invalid MOA token"))
		c.Abort()
		return
	}

	// Attach the authenticated user identifier to the request context for downstream use.
	c.Set("moa-uid", uid)
	c.Next()
}

func (s *Service) extractToken(c *gin.Context) string {
	return extractBearer(c.GetHeader(s.conf.MoaTokenHeader))
}

func extractBearer(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return header[len(prefix):]
	}
	return header
}

func isPublicPath(path string) bool {
	publicPrefixes := []string{
		"/api/common/config",
		"/api/auth/",
	}
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
