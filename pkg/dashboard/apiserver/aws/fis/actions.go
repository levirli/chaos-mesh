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
	"fmt"
	"time"

	"github.com/pkg/errors"
)

// targetSelection describes how an action selects its target resources.
type targetSelection int

const (
	// targetByArn selects the target by resource ARN (RDS).
	targetByArn targetSelection = iota
	// targetByTags selects the target by resource tags (ElastiCache). AWS FIS
	// requires tag-based selection for aws:elasticache:replicationgroup; it does
	// not support ARN selection for that resource type.
	targetByTags
)

// actionMeta is the per-action FIS orchestration metadata. Adding a new FIS
// action only requires adding a row to actionTable.
type actionMeta struct {
	// resourceType is the FIS target resource type (e.g. "aws:rds:cluster").
	resourceType string
	// targetKey is the action-specific key used to bind the action to its
	// target in the experiment template (e.g. "Clusters").
	targetKey string
	// selection is how the target resource is selected.
	selection targetSelection
	// oneshot marks irreversible actions whose Recover is a no-op.
	oneshot bool
	// actionParameters builds the FIS action parameters (action.parameters).
	actionParameters func(spec *CreateTemplateRequest) (map[string]string, error)
	// targetParameters builds the FIS target parameters (target.parameters),
	// e.g. availabilityZoneIdentifier for ElastiCache.
	targetParameters func(spec *CreateTemplateRequest) map[string]string
}

var actionTable = map[Action]actionMeta{
	RDSFailoverDBCluster: {
		resourceType: "aws:rds:cluster",
		targetKey:    "Clusters",
		selection:    targetByArn,
		oneshot:      true,
	},
	RDSRebootDBInstances: {
		resourceType: "aws:rds:db",
		targetKey:    "DBInstances",
		selection:    targetByArn,
		oneshot:      true,
		actionParameters: func(spec *CreateTemplateRequest) (map[string]string, error) {
			return map[string]string{
				"forceFailover": fmt.Sprintf("%t", spec.ForceFailover),
			}, nil
		},
	},
	ElastiCacheInterruptAZPower: {
		resourceType: "aws:elasticache:replicationgroup",
		targetKey:    "ReplicationGroups",
		selection:    targetByTags,
		oneshot:      false,
		actionParameters: func(spec *CreateTemplateRequest) (map[string]string, error) {
			iso, err := goDurationToISO8601(spec.Duration)
			if err != nil {
				return nil, err
			}
			return map[string]string{"duration": iso}, nil
		},
		targetParameters: func(spec *CreateTemplateRequest) map[string]string {
			if spec.AvailabilityZoneIdentifier == nil {
				return nil
			}
			return map[string]string{"availabilityZoneIdentifier": *spec.AvailabilityZoneIdentifier}
		},
	},
	DynamoDBPauseReplication: {
		resourceType: "aws:dynamodb:global-table",
		targetKey:    "Tables",
		selection:    targetByArn,
		oneshot:      false,
		actionParameters: func(spec *CreateTemplateRequest) (map[string]string, error) {
			iso, err := goDurationToISO8601(spec.Duration)
			if err != nil {
				return nil, err
			}
			return map[string]string{"duration": iso}, nil
		},
	},
}

// goDurationToISO8601 converts a Go duration string (e.g. "5m") into the ISO
// 8601 minute form FIS expects (e.g. "PT5M"). FIS enforces a minimum of one
// minute, so sub-minute durations are rounded up to one minute.
func goDurationToISO8601(duration *string) (string, error) {
	if duration == nil || *duration == "" {
		return "", errors.New("duration is required for this action")
	}
	d, err := time.ParseDuration(*duration)
	if err != nil {
		return "", errors.Wrapf(err, "invalid duration %q", *duration)
	}
	minutes := int(d.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("PT%dM", minutes), nil
}
