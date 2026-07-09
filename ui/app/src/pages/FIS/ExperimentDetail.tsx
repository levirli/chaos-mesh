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
import { type FISExperimentDetail, getFISexperiment } from '@/api/fis'
import Loading from '@/mui-extends/Loading'
import Paper from '@/mui-extends/Paper'
import PaperTop from '@/mui-extends/PaperTop'
import Space from '@/mui-extends/Space'
import CloseIcon from '@mui/icons-material/Close'
import { Alert, Box, Button, Chip, Typography } from '@mui/material'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { stateColor } from './Experiments'

// Field renders one read-only label/value row.
const Field = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <Box display="flex" py={1} sx={{ borderBottom: (t) => `1px solid ${t.palette.divider}` }}>
    <Typography variant="body2" color="textSecondary" sx={{ width: 200, flexShrink: 0 }}>
      {label}
    </Typography>
    <Box sx={{ flex: 1, wordBreak: 'break-all' }}>{children}</Box>
  </Box>
)

const mono = { fontFamily: 'monospace' } as const

const FISExperimentDetailPage = () => {
  const { id } = useParams()
  const navigate = useNavigate()

  const [experiment, setExperiment] = useState<FISExperimentDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!id) {
      return
    }
    getFISexperiment(id)
      .then((data) => setExperiment(data))
      .catch((err) => setError(err?.response?.data?.message ?? 'Failed to load the FIS experiment'))
      .finally(() => setLoading(false))
  }, [id])

  return (
    <Paper>
      <Box display="flex" alignItems="center" justifyContent="space-between">
        <PaperTop title="AWS FIS Experiment" h1 />
        <Button variant="outlined" startIcon={<CloseIcon />} onClick={() => navigate('/fis/experiments')}>
          关闭
        </Button>
      </Box>

      <Box mt={3}>
        {loading ? (
          <Loading />
        ) : error ? (
          <Alert severity="error">{error}</Alert>
        ) : !experiment ? (
          <Alert severity="info">Experiment not found.</Alert>
        ) : (
          <Space>
            <Field label="Experiment ID">
              <Typography variant="body2" sx={mono}>
                {experiment.id}
              </Typography>
            </Field>
            <Field label="Template ID">
              <Typography variant="body2" sx={mono}>
                {experiment.templateId || '-'}
              </Typography>
            </Field>
            <Field label="State">
              <Chip
                size="small"
                color={stateColor[experiment.state] ?? 'default'}
                label={experiment.state || '-'}
                variant="outlined"
              />
            </Field>
            {experiment.stateReason && <Field label="State reason">{experiment.stateReason}</Field>}
            {experiment.error && (
              <Field label="Error">
                <Typography variant="body2" color="error">
                  {experiment.error}
                </Typography>
              </Field>
            )}
            <Field label="Action">
              <Typography variant="body2" sx={mono}>
                {experiment.action || '-'}
              </Typography>
            </Field>
            {experiment.resourceType && <Field label="Resource type">{experiment.resourceType}</Field>}
            {experiment.targetArns && experiment.targetArns.length > 0 && (
              <Field label="Target ARNs">
                {experiment.targetArns.map((arn) => (
                  <Typography key={arn} variant="body2" sx={mono}>
                    {arn}
                  </Typography>
                ))}
              </Field>
            )}
            {experiment.resourceTags && Object.keys(experiment.resourceTags).length > 0 && (
              <Field label="Resource tags">
                {Object.entries(experiment.resourceTags).map(([k, v]) => (
                  <Chip key={k} size="small" label={`${k}=${v}`} sx={{ mr: 0.5, mb: 0.5 }} />
                ))}
              </Field>
            )}
            {experiment.actionParameters && Object.keys(experiment.actionParameters).length > 0 && (
              <Field label="Action parameters">
                {Object.entries(experiment.actionParameters).map(([k, v]) => (
                  <Chip key={k} size="small" label={`${k}=${v}`} sx={{ mr: 0.5, mb: 0.5 }} />
                ))}
              </Field>
            )}
            <Field label="Creation time">
              {experiment.creationTime ? new Date(experiment.creationTime).toLocaleString() : '-'}
            </Field>
            {experiment.startTime && (
              <Field label="Start time">{new Date(experiment.startTime).toLocaleString()}</Field>
            )}
            {experiment.endTime && <Field label="End time">{new Date(experiment.endTime).toLocaleString()}</Field>}
            {experiment.tags && Object.keys(experiment.tags).length > 0 && (
              <Field label="Tags">
                {Object.entries(experiment.tags).map(([k, v]) => (
                  <Chip key={k} size="small" label={`${k}=${v}`} sx={{ mr: 0.5, mb: 0.5 }} />
                ))}
              </Field>
            )}
          </Space>
        )}
      </Box>
    </Paper>
  )
}

export default FISExperimentDetailPage
