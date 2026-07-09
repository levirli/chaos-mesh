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
import { type FISTemplateDetail, getFIStemplate, updateFIStemplate } from '@/api/fis'
import http from '@/api/http'
import Loading from '@/mui-extends/Loading'
import Paper from '@/mui-extends/Paper'
import PaperTop from '@/mui-extends/PaperTop'
import Space from '@/mui-extends/Space'
import CloseIcon from '@mui/icons-material/Close'
import SaveOutlinedIcon from '@mui/icons-material/SaveOutlined'
import { Alert, Box, Button, Chip, Divider, MenuItem, Typography } from '@mui/material'
import { Form, Formik } from 'formik'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import * as Yup from 'yup'

import { SelectField, TextField, TextTextField } from '@/components/FormField'

import { type AWSResource, actions, availabilityZones, resourceTagsToMap, targetEndpoint } from './index'

const mono = { fontFamily: 'monospace' } as const

// Field renders one read-only label/value row.
const Field = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <Box display="flex" py={1} sx={{ borderBottom: (t) => `1px solid ${t.palette.divider}` }}>
    <Typography variant="body2" color="textSecondary" sx={{ width: 200, flexShrink: 0 }}>
      {label}
    </Typography>
    <Box sx={{ flex: 1, wordBreak: 'break-all' }}>{children}</Box>
  </Box>
)

// iso8601ToGoDuration converts the ISO 8601 minute form FIS returns (e.g. "PT5M")
// back to the Go duration string the form edits (e.g. "5m"). Falls back to the
// original string if it is not the expected shape.
const iso8601ToGoDuration = (iso: string | undefined): string => {
  if (!iso) {
    return '5m'
  }
  const m = /^PT(\d+)M$/.exec(iso)
  return m ? `${m[1]}m` : iso
}

// mapToResourceTags converts a plain string map into the TextTextField internal
// shape ({ tag0: { key, value } }) for prefilling the edit form.
const mapToResourceTags = (tags: Record<string, string> | undefined) => {
  const out: Record<string, { key: string; value: string }> = {}
  if (!tags) {
    return out
  }
  Object.entries(tags).forEach(([key, value], i) => {
    out[`tag${i}`] = { key, value }
  })
  return out
}

interface FormValues {
  description: string
  name: string
  targetArn: string
  forceFailover: string
  resourceTags: Record<string, { key: string; value: string }>
  availabilityZoneIdentifier: string
  duration: string
}

const FISTemplateDetailPage = () => {
  const { id } = useParams()
  const navigate = useNavigate()

  const [detail, setDetail] = useState<FISTemplateDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [alert, setAlert] = useState<{ type: 'success' | 'error'; message: string } | null>(null)
  const [arnOptions, setArnOptions] = useState<AWSResource[]>([])
  const [arnLoadError, setArnLoadError] = useState('')

  const meta = detail ? actions.find((a) => a.key === detail.action) : undefined

  useEffect(() => {
    if (!id) {
      return
    }
    getFIStemplate(id)
      .then((data) => setDetail(data))
      .catch((err) => setError(err?.response?.data?.message ?? 'Failed to load the FIS template'))
      .finally(() => setLoading(false))
  }, [id])

  // Load the target ARN dropdown when the (fixed) action selects by ARN.
  useEffect(() => {
    if (!meta?.targetKind) {
      return
    }
    let cancelled = false
    http
      .get<AWSResource[]>(targetEndpoint[meta.targetKind])
      .then(({ data }) => {
        if (!cancelled) {
          setArnOptions(data ?? [])
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setArnLoadError(err?.response?.data?.message ?? 'Failed to load AWS resources')
        }
      })
    return () => {
      cancelled = true
    }
  }, [meta?.targetKind])

  const renderEditForm = () => {
    if (!detail || !meta) {
      return null
    }

    const initialValues: FormValues = {
      description: detail.description ?? '',
      name: detail.tags?.Name ?? '',
      targetArn: detail.targetArn ?? '',
      forceFailover: detail.actionParameters?.forceFailover ?? 'false',
      resourceTags: mapToResourceTags(detail.resourceTags),
      availabilityZoneIdentifier: detail.targetParameters?.availabilityZoneIdentifier ?? availabilityZones[0],
      duration: iso8601ToGoDuration(detail.actionParameters?.duration),
    }

    const shape: Record<string, Yup.StringSchema> = {
      description: Yup.string().required('The description is required'),
    }
    if (meta.targetKind) {
      shape.targetArn = Yup.string().required('The target ARN is required')
    }
    if (meta.hasAvailabilityZone) {
      shape.availabilityZoneIdentifier = Yup.string().required('The availability zone is required')
    }
    if (meta.hasDuration) {
      shape.duration = Yup.string().required('The duration is required')
    }

    const handleSubmit = (values: FormValues) => {
      const spec: Record<string, unknown> = { action: meta.key, description: values.description }
      if (meta.targetKind) {
        spec.targetArn = values.targetArn
      }
      if (meta.hasForceFailover) {
        spec.forceFailover = values.forceFailover === 'true'
      }
      if (meta.hasResourceTags) {
        spec.resourceTags = resourceTagsToMap(values.resourceTags)
      }
      if (meta.hasAvailabilityZone) {
        spec.availabilityZoneIdentifier = values.availabilityZoneIdentifier
      }
      if (meta.hasDuration) {
        spec.duration = values.duration
      }

      updateFIStemplate(id!, spec as Parameters<typeof updateFIStemplate>[1])
        .then((updated) => {
          setDetail(updated)
          setAlert({ type: 'success', message: 'Updated the FIS experiment template' })
        })
        .catch((err) =>
          setAlert({ type: 'error', message: err?.response?.data?.message ?? 'Failed to update the template' }),
        )
    }

    return (
      <Formik initialValues={initialValues} validationSchema={Yup.object(shape)} onSubmit={handleSubmit}>
        {({ errors, touched }) => (
          <Form>
            <Space>
              <TextField
                name="description"
                label="Description"
                helperText={
                  touched.description && errors.description
                    ? (errors.description as string)
                    : 'The experiment description'
                }
                error={Boolean(touched.description && errors.description)}
              />

              <TextField
                name="name"
                label="Name"
                disabled
                helperText="The template name (Name tag) cannot be changed after creation."
              />

              {meta.targetKind && (
                <SelectField
                  name="targetArn"
                  label={meta.targetLabel}
                  helperText={
                    touched.targetArn && errors.targetArn
                      ? (errors.targetArn as string)
                      : arnLoadError || 'Populated from AWS via the /api/aws listing endpoint'
                  }
                  error={Boolean((touched.targetArn && errors.targetArn) || arnLoadError)}
                >
                  {arnOptions.map((r) => (
                    <MenuItem key={r.arn} value={r.arn}>
                      {r.identifier || r.arn}
                    </MenuItem>
                  ))}
                </SelectField>
              )}

              {meta.hasForceFailover && (
                <SelectField
                  name="forceFailover"
                  label="Force failover"
                  helperText="Whether to force a failover during the reboot"
                >
                  <MenuItem value="false">false</MenuItem>
                  <MenuItem value="true">true</MenuItem>
                </SelectField>
              )}

              {meta.hasResourceTags && (
                <TextTextField
                  name="resourceTags"
                  label="Resource tags"
                  helperText="Tags selecting the target ElastiCache replication group"
                />
              )}

              {meta.hasAvailabilityZone && (
                <SelectField
                  name="availabilityZoneIdentifier"
                  label="Availability zone"
                  helperText={
                    touched.availabilityZoneIdentifier && errors.availabilityZoneIdentifier
                      ? (errors.availabilityZoneIdentifier as string)
                      : 'The availability zone to interrupt. Must match the configured region'
                  }
                  error={Boolean(touched.availabilityZoneIdentifier && errors.availabilityZoneIdentifier)}
                >
                  {availabilityZones.map((az) => (
                    <MenuItem key={az} value={az}>
                      {az}
                    </MenuItem>
                  ))}
                </SelectField>
              )}

              {meta.hasDuration && (
                <TextField
                  name="duration"
                  label="Duration"
                  helperText={
                    touched.duration && errors.duration
                      ? (errors.duration as string)
                      : 'How long the fault lasts, e.g. 5m'
                  }
                  error={Boolean(touched.duration && errors.duration)}
                />
              )}
            </Space>

            <Box display="flex" gap={2} mt={3}>
              <Button type="submit" variant="contained" startIcon={<SaveOutlinedIcon />}>
                更新
              </Button>
              <Button variant="outlined" startIcon={<CloseIcon />} onClick={() => navigate('/fis')}>
                返回
              </Button>
            </Box>
          </Form>
        )}
      </Formik>
    )
  }

  return (
    <Paper>
      <Box display="flex" alignItems="center" justifyContent="space-between">
        <PaperTop title="AWS FIS Experiment Template" h1 />
        <Button variant="outlined" startIcon={<CloseIcon />} onClick={() => navigate('/fis')}>
          返回
        </Button>
      </Box>

      <Box mt={3}>
        {loading ? (
          <Loading />
        ) : error ? (
          <Alert severity="error">{error}</Alert>
        ) : !detail ? (
          <Alert severity="info">Template not found.</Alert>
        ) : (
          <>
            <Space>
              <Field label="Template ID">
                <Typography variant="body2" sx={mono}>
                  {detail.id}
                </Typography>
              </Field>
              <Field label="Action">
                <Typography variant="body2" sx={mono}>
                  {detail.action || '-'}
                </Typography>
              </Field>
              {detail.resourceType && <Field label="Resource type">{detail.resourceType}</Field>}
              {detail.roleArn && (
                <Field label="Role ARN">
                  <Typography variant="body2" sx={mono}>
                    {detail.roleArn}
                  </Typography>
                </Field>
              )}
              {detail.tags && Object.keys(detail.tags).length > 0 && (
                <Field label="Tags">
                  {Object.entries(detail.tags).map(([k, v]) => (
                    <Chip key={k} size="small" label={`${k}=${v}`} sx={{ mr: 0.5, mb: 0.5 }} />
                  ))}
                </Field>
              )}
              <Field label="Creation time">
                {detail.creationTime ? new Date(detail.creationTime).toLocaleString() : '-'}
              </Field>
              <Field label="Last update time">
                {detail.lastUpdateTime ? new Date(detail.lastUpdateTime).toLocaleString() : '-'}
              </Field>
            </Space>

            {alert && (
              <Alert severity={alert.type} sx={{ mt: 3 }} onClose={() => setAlert(null)}>
                {alert.message}
              </Alert>
            )}

            <Divider sx={{ my: 4 }} />

            {meta ? (
              <>
                <PaperTop title="Update template" />
                <Box mt={2}>{renderEditForm()}</Box>
              </>
            ) : (
              <Alert severity="info">
                This template uses an action not managed by the dashboard, so it is read-only here.
              </Alert>
            )}
          </>
        )}
      </Box>
    </Paper>
  )
}

export default FISTemplateDetailPage
