/*
 * Copyright 2021 Chaos Mesh Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
import http from '@/api/http'

// FISAction is the AWS FIS action id (e.g. aws:rds:failover-db-cluster).
export type FISAction =
  | 'aws:rds:failover-db-cluster'
  | 'aws:rds:reboot-db-instances'
  | 'aws:elasticache:replicationgroup-interrupt-az-power'
  | 'aws:dynamodb:global-table-pause-replication'

export interface FISCreateTemplateRequest {
  action: FISAction
  description: string
  name?: string
  targetArn?: string
  forceFailover?: boolean
  resourceTags?: Record<string, string>
  availabilityZoneIdentifier?: string
  duration?: string
}

export interface FISTemplateSummary {
  id: string
  description: string
  // Action is empty when AWS ListExperimentTemplates did not return the
  // actions map; the UI may render it blank or fetch details on demand.
  action?: string
  creationTime: string
  tags?: Record<string, string>
}

export interface FISExperimentSummary {
  id: string
  templateId?: string
  state: string
  creationTime: string
  tags?: Record<string, string>
}

// FISTemplateDetail is the full template returned by GET /aws/fis/templates/:id
// (built from GetExperimentTemplate, so action/target are available).
export interface FISTemplateDetail {
  id: string
  description: string
  action: string
  resourceType?: string
  selectionMode?: string
  targetArn?: string
  resourceTags?: Record<string, string>
  actionParameters?: Record<string, string>
  targetParameters?: Record<string, string>
  roleArn?: string
  tags?: Record<string, string>
  creationTime: string
  lastUpdateTime: string
}

// FISExperimentDetail is the full experiment returned by GET /aws/fis/experiments/:id.
export interface FISExperimentDetail {
  id: string
  templateId?: string
  state: string
  stateReason?: string
  error?: string
  action?: string
  resourceType?: string
  targetArns?: string[]
  resourceTags?: Record<string, string>
  actionParameters?: Record<string, string>
  tags?: Record<string, string>
  creationTime: string
  startTime?: string
  endTime?: string
}

export interface FISCreateTemplateResponse {
  id: string
}

export interface FISStartExperimentResponse {
  id: string
}

export const listFIStemplates = () =>
  http.get<FISTemplateSummary[]>('/aws/fis/templates').then(({ data }) => data ?? [])

export const createFIStemplate = (body: FISCreateTemplateRequest) =>
  http.post<FISCreateTemplateResponse>('/aws/fis/templates', body).then(({ data }) => data)

export const getFIStemplate = (id: string) =>
  http.get<FISTemplateDetail>(`/aws/fis/templates/${id}`).then(({ data }) => data)

export const updateFIStemplate = (id: string, body: FISCreateTemplateRequest) =>
  http.put<FISTemplateDetail>(`/aws/fis/templates/${id}`, body).then(({ data }) => data)

export const deleteFIStemplate = (id: string) => http.delete(`/aws/fis/templates/${id}`)

export const startFISexperiment = (templateId: string) =>
  http.post<FISStartExperimentResponse>(`/aws/fis/templates/${templateId}/start`).then(({ data }) => data)

export const listFISexperiments = () =>
  http.get<FISExperimentSummary[]>('/aws/fis/experiments').then(({ data }) => data ?? [])

export const getFISexperiment = (id: string) =>
  http.get<FISExperimentDetail>(`/aws/fis/experiments/${id}`).then(({ data }) => data)

export const stopFISexperiment = (id: string) => http.post(`/aws/fis/experiments/${id}/stop`)
