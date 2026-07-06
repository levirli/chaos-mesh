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
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
)

const defaultUserInfoEndpoint = "https://api-ms-portal.infra.catdelta.com/user/info"
const tokenHeaderName = "Moa-Token"

// RemoteValidator calls the MOA user info endpoint and validates the token by
// comparing the returned project_id with the configured project ID.
type RemoteValidator struct {
	endpoint   string
	projectID  string
	httpClient *http.Client
}

// userInfoResponse matches the JSON returned by the MOA user info endpoint.
type userInfoResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID        int    `json:"id"`
		UID       int64  `json:"uid"`
		Name      string `json:"name"`
		Nick      string `json:"nick"`
		Email     string `json:"email"`
		ProjectID int    `json:"project_id"`
	} `json:"data"`
	Time int64 `json:"time"`
}

// NewRemoteValidator creates a validator that talks to the MOA user info endpoint.
func NewRemoteValidator(projectID string, client *http.Client) *RemoteValidator {
	if client == nil {
		client = http.DefaultClient
	}
	return &RemoteValidator{
		endpoint:   defaultUserInfoEndpoint,
		projectID:  projectID,
		httpClient: client,
	}
}

// Validate calls the MOA user info endpoint and returns the user UID if the token
// belongs to the configured project.
func (v *RemoteValidator) Validate(token string) (string, error) {
	if token == "" {
		return "", errors.New("empty token")
	}

	req, err := http.NewRequest(http.MethodGet, v.endpoint, nil)
	if err != nil {
		return "", errors.Wrap(err, "build MOA user info request")
	}
	req.Header.Set(tokenHeaderName, token)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "call MOA user info endpoint")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read MOA user info response")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MOA user info returned status %d: %s", resp.StatusCode, string(body))
	}

	var info userInfoResponse
	if err := json.Unmarshal(body, &info); err != nil {
		return "", errors.Wrap(err, "decode MOA user info response")
	}

	if info.Code != 0 {
		return "", fmt.Errorf("MOA user info returned code %d: %s", info.Code, info.Message)
	}

	expectedProjectID := v.projectID
	if expectedProjectID == "" {
		expectedProjectID = "1600"
	}

	actualProjectID := fmt.Sprintf("%d", info.Data.ProjectID)
	if actualProjectID != expectedProjectID {
		return "", fmt.Errorf("MOA project_id mismatch: expected %s, got %s", expectedProjectID, actualProjectID)
	}

	return fmt.Sprintf("%d", info.Data.UID), nil
}
