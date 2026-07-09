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

// Package fis provides dashboard-side endpoints for managing AWS FIS
// experiment templates and listing experiments. The dashboard talks to AWS
// FIS directly using the deployment-level fixed AWSFISConfig; templates
// persist in AWS FIS and are not represented as Kubernetes resources.
package fis

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaos-mesh/chaos-mesh/pkg/config"
)

// Service serves the AWS FIS template and experiment endpoints.
type Service struct {
	kubeCli client.Client
	conf    *config.ChaosDashboardConfig
	logger  logr.Logger
}

// NewService returns a FIS service instance.
func NewService(
	conf *config.ChaosDashboardConfig,
	kubeCli client.Client,
	logger logr.Logger,
) *Service {
	return &Service{
		conf:    conf,
		kubeCli: kubeCli,
		logger:  logger.WithName("fis-api"),
	}
}
