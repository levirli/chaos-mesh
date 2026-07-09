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
import { type FISExperimentSummary, listFISexperiments, stopFISexperiment } from '@/api/fis'
import ConfirmDialog from '@/mui-extends/ConfirmDialog'
import Loading from '@/mui-extends/Loading'
import Paper from '@/mui-extends/Paper'
import PaperTop from '@/mui-extends/PaperTop'
import FormatListBulletedIcon from '@mui/icons-material/FormatListBulleted'
import StopCircleOutlinedIcon from '@mui/icons-material/StopCircleOutlined'
import {
  Alert,
  Box,
  Button,
  Chip,
  Link,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
} from '@mui/material'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'

export const stateColor: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'error' | 'info'> = {
  pending: 'warning',
  initiating: 'info',
  running: 'primary',
  completed: 'success',
  stopping: 'warning',
  stopped: 'default',
  failed: 'error',
  cancelled: 'default',
}

// stoppableStates are the non-terminal states where StopExperiment is valid, so
// the UI shows a Stop button. Terminal states (completed/stopped/failed/...) are
// omitted.
const stoppableStates = new Set(['pending', 'initiating', 'running'])

const FISExperiments = () => {
  const navigate = useNavigate()

  const [experiments, setExperiments] = useState<FISExperimentSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [alert, setAlert] = useState<{ type: 'success' | 'error'; message: string } | null>(null)
  const [stopTarget, setStopTarget] = useState<FISExperimentSummary | null>(null)

  const refresh = () => {
    setLoading(true)
    setError('')
    listFISexperiments()
      .then((data) => setExperiments(data ?? []))
      .catch((err) => setError(err?.response?.data?.message ?? 'Failed to load FIS experiments'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    refresh()
  }, [])

  const handleStop = () => {
    if (!stopTarget) {
      return
    }
    const id = stopTarget.id
    stopFISexperiment(id)
      .then(() => {
        setAlert({ type: 'success', message: `Stopping experiment ${id}` })
        refresh()
      })
      .catch((err) => {
        const status = err?.response?.status
        setAlert({ type: 'error', message: err?.response?.data?.message ?? 'Failed to stop experiment' })
        if (status === 404) {
          refresh()
        }
      })
      .finally(() => setStopTarget(null))
  }

  return (
    <Paper>
      <Box display="flex" alignItems="center" justifyContent="space-between">
        <PaperTop
          title="AWS FIS Experiments"
          subtitle="Running and completed experiments from AWS Fault Injection Simulator"
          h1
        />
        <Button variant="outlined" startIcon={<FormatListBulletedIcon />} onClick={() => navigate('/fis')}>
          Experiment Templates
        </Button>
      </Box>

      <Box mt={3}>
        {loading ? (
          <Loading />
        ) : error ? (
          <Alert severity="error">{error}</Alert>
        ) : experiments.length === 0 ? (
          <Alert severity="info">No experiments found.</Alert>
        ) : (
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>Experiment ID</TableCell>
                  <TableCell>Template ID</TableCell>
                  <TableCell>State</TableCell>
                  <TableCell>Creation Time</TableCell>
                  <TableCell>Tags</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {experiments.map((e) => (
                  <TableRow key={e.id} hover>
                    <TableCell sx={{ fontFamily: 'monospace' }}>
                      <Link
                        component="button"
                        underline="hover"
                        sx={{ fontFamily: 'monospace' }}
                        onClick={() => navigate(`/fis/experiments/${e.id}`)}
                      >
                        {e.id}
                      </Link>
                    </TableCell>
                    <TableCell sx={{ fontFamily: 'monospace' }}>{e.templateId || '-'}</TableCell>
                    <TableCell>
                      <Chip
                        size="small"
                        color={stateColor[e.state] ?? 'default'}
                        label={e.state || '-'}
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell>{e.creationTime ? new Date(e.creationTime).toLocaleString() : '-'}</TableCell>
                    <TableCell>
                      {e.tags &&
                        Object.entries(e.tags).map(([k, v]) => (
                          <Chip key={k} size="small" label={`${k}=${v}`} sx={{ mr: 0.5, mb: 0.5 }} />
                        ))}
                    </TableCell>
                    <TableCell align="right">
                      {stoppableStates.has(e.state) && (
                        <Button
                          size="small"
                          variant="outlined"
                          color="warning"
                          startIcon={<StopCircleOutlinedIcon />}
                          onClick={() => setStopTarget(e)}
                        >
                          Stop
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Box>

      {alert && (
        <Alert severity={alert.type} sx={{ mt: 3 }} onClose={() => setAlert(null)}>
          {alert.message}
        </Alert>
      )}

      <ConfirmDialog
        open={stopTarget !== null}
        close={() => setStopTarget(null)}
        title="Stop experiment"
        description={
          stopTarget ? (
            <>
              Stop running experiment <code>{stopTarget.id}</code>? This interrupts the fault injection in AWS FIS.
            </>
          ) : (
            ''
          )
        }
        confirmText="Stop"
        onConfirm={handleStop}
      />
    </Paper>
  )
}

export default FISExperiments
