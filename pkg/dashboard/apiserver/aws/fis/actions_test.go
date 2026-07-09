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

import "testing"

func strptr(s string) *string { return &s }

func TestGoDurationToISO8601(t *testing.T) {
	cases := []struct {
		name    string
		in      *string
		want    string
		wantErr bool
	}{
		{name: "minutes", in: strptr("5m"), want: "PT5M"},
		{name: "hour", in: strptr("1h"), want: "PT60M"},
		{name: "sub-minute rounds up to 1", in: strptr("30s"), want: "PT1M"},
		{name: "nil is error", in: nil, wantErr: true},
		{name: "empty is error", in: strptr(""), wantErr: true},
		{name: "invalid is error", in: strptr("abc"), wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := goDurationToISO8601(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("want %q, got %q", c.want, got)
			}
		})
	}
}

func TestActionTableCoverage(t *testing.T) {
	want := map[Action]struct {
		resourceType string
		targetKey    string
		selection    targetSelection
		oneshot      bool
	}{
		RDSFailoverDBCluster:        {"aws:rds:cluster", "Clusters", targetByArn, true},
		RDSRebootDBInstances:        {"aws:rds:db", "DBInstances", targetByArn, true},
		ElastiCacheInterruptAZPower: {"aws:elasticache:replicationgroup", "ReplicationGroups", targetByTags, false},
		DynamoDBPauseReplication:    {"aws:dynamodb:global-table", "Tables", targetByArn, false},
	}

	if len(actionTable) != len(want) {
		t.Fatalf("actionTable has %d entries, want %d", len(actionTable), len(want))
	}
	for action, w := range want {
		meta, ok := actionTable[action]
		if !ok {
			t.Fatalf("missing action %s in actionTable", action)
		}
		if meta.resourceType != w.resourceType || meta.targetKey != w.targetKey ||
			meta.selection != w.selection || meta.oneshot != w.oneshot {
			t.Errorf("action %s metadata mismatch: got %+v", action, meta)
		}
	}
}

func TestActionParameters(t *testing.T) {
	// reboot forceFailover=true
	reboot := actionTable[RDSRebootDBInstances]
	forceTrue := true
	params, err := reboot.actionParameters(&CreateTemplateRequest{
		ForceFailover: forceTrue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["forceFailover"] != "true" {
		t.Errorf("forceFailover: want true, got %q", params["forceFailover"])
	}

	// interrupt-az-power duration -> ISO8601 + AZ target parameter
	az := actionTable[ElastiCacheInterruptAZPower]
	spec := &CreateTemplateRequest{
		Duration:                    strptr("5m"),
		AvailabilityZoneIdentifier: strptr("ap-southeast-1a"),
	}
	aparams, err := az.actionParameters(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aparams["duration"] != "PT5M" {
		t.Errorf("duration: want PT5M, got %q", aparams["duration"])
	}
	tparams := az.targetParameters(spec)
	if tparams["availabilityZoneIdentifier"] != "ap-southeast-1a" {
		t.Errorf("availabilityZoneIdentifier: want ap-southeast-1a, got %q", tparams["availabilityZoneIdentifier"])
	}
}
