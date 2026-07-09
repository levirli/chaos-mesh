// Copyright 2021 Chaos Mesh Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package fis

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/chaos-mesh/chaos-mesh/pkg/config"
)

// stringReader returns an io.Reader for the given string, for HTTP request
// bodies in tests.
func stringReader(s string) *strings.Reader { return strings.NewReader(s) }

// TestFISRoutesRegistered verifies the five /aws/fis/* endpoints are mounted
// and reachable (i.e. not 404). Without AWS fixed configuration the handlers
// error internally, but the routes must resolve.
func TestFISRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &Service{
		kubeCli: fake.NewClientBuilder().Build(),
		conf:    &config.ChaosDashboardConfig{},
		logger:  logr.Discard(),
	}

	r := gin.New()
	Register(r.Group("/api"), svc)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/aws/fis/templates"},
		{http.MethodPost, "/api/aws/fis/templates"},
		{http.MethodGet, "/api/aws/fis/templates/EXT123"},
		{http.MethodPut, "/api/aws/fis/templates/EXT123"},
		{http.MethodDelete, "/api/aws/fis/templates/EXT123"},
		{http.MethodPost, "/api/aws/fis/templates/EXT123/start"},
		{http.MethodGet, "/api/aws/fis/experiments"},
		{http.MethodGet, "/api/aws/fis/experiments/EXP123"},
		{http.MethodPost, "/api/aws/fis/experiments/EXP123/stop"},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tt.method, tt.path, nil)
		r.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatalf("route %s %s not registered (got 404)", tt.method, tt.path)
		}
	}
}

// TestCreateTemplateValidation covers the validation paths in
// POST /aws/fis/templates: unsupported action, missing targetArn for byArn
// actions, missing resourceTags for byTags actions. These are checked before
// any AWS call, so no FIS client is needed.
func TestCreateTemplateValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &Service{
		kubeCli: fake.NewClientBuilder().Build(),
		conf: &config.ChaosDashboardConfig{
			AWSFIS: config.AWSFISConfig{
				Region:  "ap-southeast-1",
				RoleArn: "arn:aws:iam::123456789012:role/FISRole",
			},
		},
		logger: logr.Discard(),
	}

	r := gin.New()
	Register(r.Group("/api"), svc)

	cases := []struct {
		name string
		body string
	}{
		{
			name: "unsupported action",
			body: `{"action":"aws:unknown:foo","description":"d"}`,
		},
		{
			name: "missing targetArn for RDS reboot",
			body: `{"action":"aws:rds:reboot-db-instances","description":"d"}`,
		},
		{
			name: "missing resourceTags for ElastiCache",
			body: `{"action":"aws:elasticache:replicationgroup-interrupt-az-power","description":"d","duration":"5m"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/aws/fis/templates", stringReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (body=%s)", w.Code, w.Body.String())
			}
		})
	}
}

// TestUpdateTemplateValidation covers the validation paths in
// PUT /aws/fis/templates/:id that run before any AWS call: unsupported action
// and missing targetArn for byArn actions. No FIS client is needed.
func TestUpdateTemplateValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &Service{
		kubeCli: fake.NewClientBuilder().Build(),
		conf: &config.ChaosDashboardConfig{
			AWSFIS: config.AWSFISConfig{Region: "ap-southeast-1"},
		},
		logger: logr.Discard(),
	}

	r := gin.New()
	Register(r.Group("/api"), svc)

	cases := []struct {
		name string
		body string
	}{
		{
			name: "unsupported action",
			body: `{"action":"aws:unknown:foo","description":"d"}`,
		},
		{
			name: "missing targetArn for RDS reboot",
			body: `{"action":"aws:rds:reboot-db-instances","description":"d"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/aws/fis/templates/EXT123", stringReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (body=%s)", w.Code, w.Body.String())
			}
		})
	}
}

// TestCreateTemplateMissingRoleArn verifies the handler rejects creation when
// the deployment-level RoleArn is unset, before any AWS call.
func TestCreateTemplateMissingRoleArn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &Service{
		kubeCli: fake.NewClientBuilder().Build(),
		conf: &config.ChaosDashboardConfig{
			AWSFIS: config.AWSFISConfig{
				Region: "ap-southeast-1",
				// RoleArn intentionally empty
			},
		},
		logger: logr.Discard(),
	}

	r := gin.New()
	Register(r.Group("/api"), svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/aws/fis/templates",
		stringReader(`{"action":"aws:rds:reboot-db-instances","description":"d","targetArn":"arn:aws:rds:ap-southeast-1:123456789012:db:mydb"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (body=%s)", w.Code, w.Body.String())
	}
}
