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
import { type FISTemplateSummary, deleteFIStemplate, listFIStemplates, startFISexperiment } from '@/api/fis'
import ConfirmDialog from '@/mui-extends/ConfirmDialog'
import Loading from '@/mui-extends/Loading'
import Paper from '@/mui-extends/Paper'
import PaperTop from '@/mui-extends/PaperTop'
import AddIcon from '@mui/icons-material/Add'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import FormatListBulletedIcon from '@mui/icons-material/FormatListBulleted'
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline'
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
  Typography,
} from '@mui/material'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'

const FISList = () => {
  const navigate = useNavigate()

  const [templates, setTemplates] = useState<FISTemplateSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [alert, setAlert] = useState<{ type: 'success' | 'error'; message: string } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<FISTemplateSummary | null>(null)

  const refresh = () => {
    setLoading(true)
    setError('')
    listFIStemplates()
      .then((data) => setTemplates(data ?? []))
      .catch((err) => setError(err?.response?.data?.message ?? 'Failed to load FIS templates'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    refresh()
  }, [])

  const handleStart = (t: FISTemplateSummary) => {
    startFISexperiment(t.id)
      .then(() => {
        setAlert({ type: 'success', message: `Started experiment from template ${t.id}` })
        setTimeout(() => navigate('/fis/experiments'), 600)
      })
      .catch((err) =>
        setAlert({ type: 'error', message: err?.response?.data?.message ?? 'Failed to start experiment' }),
      )
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    const id = deleteTarget.id
    deleteFIStemplate(id)
      .then(() => {
        setAlert({ type: 'success', message: `Deleted template ${id}` })
        setTemplates((prev) => prev.filter((t) => t.id !== id))
      })
      .catch((err) => {
        const status = err?.response?.status
        const msg = err?.response?.data?.message ?? 'Failed to delete template'
        setAlert({ type: 'error', message: msg })
        if (status === 404) {
          // Template gone out-of-band; refresh list to reflect reality.
          refresh()
        }
      })
      .finally(() => setDeleteTarget(null))
  }

  return (
    <Paper>
      <Box display="flex" alignItems="center" justifyContent="space-between">
        <PaperTop title="AWS FIS Experiment Templates" h1 />
        <Box display="flex" gap={2}>
          <Button
            variant="outlined"
            startIcon={<FormatListBulletedIcon />}
            onClick={() => navigate('/fis/experiments')}
          >
            Experiments
          </Button>
          <Button variant="contained" startIcon={<AddIcon />} onClick={() => navigate('/fis/new')}>
            New Experiment Template
          </Button>
        </Box>
      </Box>

      <Box mt={3}>
        {loading ? (
          <Loading />
        ) : error ? (
          <Alert severity="error">{error}</Alert>
        ) : templates.length === 0 ? (
          <Alert severity="info">
            No experiment templates yet. Click <strong>New Experiment Template</strong> to create one.
          </Alert>
        ) : (
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>ID</TableCell>
                  <TableCell>Description</TableCell>
                  <TableCell>Creation Time</TableCell>
                  <TableCell>Tags</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {templates.map((t) => (
                  <TableRow key={t.id} hover>
                    <TableCell sx={{ fontFamily: 'monospace' }}>
                      <Link
                        component="button"
                        underline="hover"
                        sx={{ fontFamily: 'monospace' }}
                        onClick={() => navigate(`/fis/templates/${t.id}`)}
                      >
                        {t.id}
                      </Link>
                    </TableCell>
                    <TableCell>{t.description || '-'}</TableCell>
                    <TableCell>{t.creationTime ? new Date(t.creationTime).toLocaleString() : '-'}</TableCell>
                    <TableCell>
                      {t.tags &&
                        Object.entries(t.tags).map(([k, v]) => (
                          <Chip key={k} size="small" label={`${k}=${v}`} sx={{ mr: 0.5, mb: 0.5 }} />
                        ))}
                    </TableCell>
                    <TableCell align="right">
                      <Box display="flex" gap={1} justifyContent="flex-end">
                        <Button
                          size="small"
                          variant="outlined"
                          color="primary"
                          startIcon={<PlayCircleOutlineIcon />}
                          onClick={() => handleStart(t)}
                        >
                          Start
                        </Button>
                        <Button
                          size="small"
                          variant="outlined"
                          color="error"
                          startIcon={<DeleteOutlineIcon />}
                          onClick={() => setDeleteTarget(t)}
                        >
                          Delete
                        </Button>
                      </Box>
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
        open={deleteTarget !== null}
        close={() => setDeleteTarget(null)}
        title="Delete experiment template"
        description={
          deleteTarget ? (
            <>
              Delete template <code>{deleteTarget.id}</code> from AWS FIS? This action cannot be undone.
            </>
          ) : (
            ''
          )
        }
        confirmText="Delete"
        onConfirm={handleDelete}
      />
    </Paper>
  )
}

export default FISList
