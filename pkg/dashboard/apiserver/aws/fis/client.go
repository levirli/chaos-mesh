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
	"context"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/fis"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// newFISClient builds a request-scoped FIS client from the deployment-level
// fixed configuration. Credentials are read from the configured K8s Secret
// when SecretName is set; otherwise the AWS SDK default credential chain
// applies. Adapted from the legacy awsfischaos controller's newFISClient and
// the dashboard's existing newRDSClient.
func (s *Service) newFISClient(ctx context.Context) (*fis.Client, error) {
	fisCfg := s.conf.AWSFIS
	if fisCfg.Region == "" {
		return nil, errors.New("AWS FIS fixed configuration missing: AWSFIS_REGION must be set")
	}

	opts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(fisCfg.Region),
	}

	if fisCfg.SecretName != "" {
		secret := &v1.Secret{}
		if err := s.kubeCli.Get(ctx, types.NamespacedName{
			Name:      fisCfg.SecretName,
			Namespace: fisCfg.SecretNamespace,
		}, secret); err != nil {
			return nil, errors.Wrap(err, "get AWS credential secret")
		}
		opts = append(opts, awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			string(secret.Data["aws_access_key_id"]),
			string(secret.Data["aws_secret_access_key"]),
			string(secret.Data["aws_session_token"]),
		)))
	}

	cfg, err := awscfg.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "load AWS SDK config")
	}
	return fis.NewFromConfig(cfg), nil
}
