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

import "time"

// Action is the AWS FIS action id (e.g. "aws:rds:failover-db-cluster").
// Mirrors the values previously defined in api/v1alpha1.AWSFISChaosAction.
type Action string

const (
	// RDSFailoverDBCluster triggers an RDS cluster failover. One-shot / irreversible.
	RDSFailoverDBCluster Action = "aws:rds:failover-db-cluster"
	// RDSRebootDBInstances reboots the target RDS instance. One-shot / irreversible.
	RDSRebootDBInstances Action = "aws:rds:reboot-db-instances"
	// ElastiCacheInterruptAZPower interrupts power of one AZ of an ElastiCache replication group.
	ElastiCacheInterruptAZPower Action = "aws:elasticache:replicationgroup-interrupt-az-power"
	// DynamoDBPauseReplication pauses cross-region replication of a DynamoDB global table.
	DynamoDBPauseReplication Action = "aws:dynamodb:global-table-pause-replication"
)

// CreateTemplateRequest is the request body for POST /aws/fis/templates. Field
// shapes mirror the legacy AWSFISChaosSpec so the existing UI form submits
// unchanged.
type CreateTemplateRequest struct {
	// Action is the FIS action id.
	Action Action `json:"action"`
	// Description is the human-readable template description.
	Description string `json:"description"`
	// Name is the template's Name tag. When empty, a random name is generated.
	// It can only be set at creation time (UpdateExperimentTemplate cannot change tags).
	Name string `json:"name,omitempty"`
	// TargetArn is the ARN of the target resource for RDS actions.
	TargetArn *string `json:"targetArn,omitempty"`
	// ForceFailover, when true, forces a failover across availability zones
	// during an RDS reboot.
	ForceFailover bool `json:"forceFailover,omitempty"`
	// ResourceTags selects the target ElastiCache replication group by tags.
	ResourceTags map[string]string `json:"resourceTags,omitempty"`
	// AvailabilityZoneIdentifier is the AZ whose power is interrupted for
	// ElastiCache actions.
	AvailabilityZoneIdentifier *string `json:"availabilityZoneIdentifier,omitempty"`
	// Duration is how long the fault lasts, as a Go duration string (e.g. "5m").
	Duration *string `json:"duration,omitempty"`
}

// TemplateSummary is one row of GET /aws/fis/templates.
type TemplateSummary struct {
	// ID is the FIS template id (e.g. "EXT1234567890").
	ID string `json:"id"`
	// Description is the template description.
	Description string `json:"description"`
	// Action is the actionId extracted from the template's actions map.
	Action string `json:"action"`
	// CreationTime is the AWS-returned creation timestamp.
	CreationTime time.Time `json:"creationTime"`
	// Tags are the template's tags.
	Tags map[string]string `json:"tags,omitempty"`
}

// ExperimentSummary is one row of GET /aws/fis/experiments.
type ExperimentSummary struct {
	// ID is the FIS experiment id.
	ID string `json:"id"`
	// TemplateID is the source template id, extracted from the
	// chaos-mesh/template-id tag when present.
	TemplateID string `json:"templateId,omitempty"`
	// State is the experiment state (pending / initiating / running / completed / failed / stopped).
	State string `json:"state"`
	// CreationTime is the AWS-returned creation timestamp.
	CreationTime time.Time `json:"creationTime"`
	// Tags are the experiment's tags.
	Tags map[string]string `json:"tags,omitempty"`
}

// TemplateDetail is the response body for GET /aws/fis/templates/:id. Unlike
// TemplateSummary it is built from GetExperimentTemplate, which returns the full
// actions/targets maps, so it can expose the action and target.
type TemplateDetail struct {
	// ID is the FIS template id.
	ID string `json:"id"`
	// Description is the template description.
	Description string `json:"description"`
	// Action is the actionId of the template's (single) action.
	Action string `json:"action"`
	// ResourceType is the target resource type (e.g. "aws:rds:db").
	ResourceType string `json:"resourceType,omitempty"`
	// SelectionMode is the target selection mode (e.g. "ALL").
	SelectionMode string `json:"selectionMode,omitempty"`
	// TargetArn is the first target resource ARN, when the target selects by ARN.
	TargetArn string `json:"targetArn,omitempty"`
	// ResourceTags are the target's resource tags, when the target selects by tags.
	ResourceTags map[string]string `json:"resourceTags,omitempty"`
	// ActionParameters are the action's parameters (e.g. forceFailover, duration).
	ActionParameters map[string]string `json:"actionParameters,omitempty"`
	// TargetParameters are the target's parameters (e.g. availabilityZoneIdentifier).
	TargetParameters map[string]string `json:"targetParameters,omitempty"`
	// RoleArn is the IAM role FIS assumes to run the experiment.
	RoleArn string `json:"roleArn,omitempty"`
	// Tags are the template's tags.
	Tags map[string]string `json:"tags,omitempty"`
	// CreationTime is the AWS-returned creation timestamp.
	CreationTime time.Time `json:"creationTime"`
	// LastUpdateTime is the AWS-returned last-update timestamp.
	LastUpdateTime time.Time `json:"lastUpdateTime"`
}

// ExperimentDetail is the response body for GET /aws/fis/experiments/:id, built
// from GetExperiment.
type ExperimentDetail struct {
	// ID is the FIS experiment id.
	ID string `json:"id"`
	// TemplateID is the source experiment template id.
	TemplateID string `json:"templateId,omitempty"`
	// State is the experiment state status.
	State string `json:"state"`
	// StateReason is the AWS-provided reason for the current state.
	StateReason string `json:"stateReason,omitempty"`
	// Error is the AWS-provided error detail when the experiment failed.
	Error string `json:"error,omitempty"`
	// Action is the actionId of the experiment's (single) action.
	Action string `json:"action,omitempty"`
	// ResourceType is the target resource type.
	ResourceType string `json:"resourceType,omitempty"`
	// TargetArns are the target resource ARNs.
	TargetArns []string `json:"targetArns,omitempty"`
	// ResourceTags are the target's resource tags.
	ResourceTags map[string]string `json:"resourceTags,omitempty"`
	// ActionParameters are the action's parameters.
	ActionParameters map[string]string `json:"actionParameters,omitempty"`
	// Tags are the experiment's tags.
	Tags map[string]string `json:"tags,omitempty"`
	// CreationTime is the AWS-returned creation timestamp.
	CreationTime time.Time `json:"creationTime"`
	// StartTime is when the experiment started, if it has.
	StartTime *time.Time `json:"startTime,omitempty"`
	// EndTime is when the experiment ended, if it has.
	EndTime *time.Time `json:"endTime,omitempty"`
}

// createTemplateResponse is the response body for POST /aws/fis/templates.
type createTemplateResponse struct {
	ID string `json:"id"`
}

// startExperimentResponse is the response body for POST /aws/fis/templates/:id/start.
type startExperimentResponse struct {
	ID string `json:"id"`
}

// stopExperimentResponse is the response body for POST /aws/fis/experiments/:id/stop.
type stopExperimentResponse struct {
	ID string `json:"id"`
}
