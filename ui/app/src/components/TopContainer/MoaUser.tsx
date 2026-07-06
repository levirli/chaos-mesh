/*
 * Copyright 2025 Chaos Mesh Authors.
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
import { resetMOAAuthentication } from '@/api/interceptors'
import { Stale } from '@/api/queryUtils'
import { useGetCommonConfig } from '@/openapi'
import { useAuthActions, useAuthStore } from '@/zustand/auth'
import LogoutIcon from '@mui/icons-material/Logout'
import { Avatar, Box, Divider, IconButton, ListItemIcon, Menu, MenuItem, Typography } from '@mui/material'
import { useState } from 'react'

const MoaUser = () => {
  const { data: config } = useGetCommonConfig({
    query: {
      enabled: false,
      staleTime: Stale.DAY,
    },
  })

  const moaUser = useAuthStore((state) => state.moaUser)
  const { removeMoaToken } = useAuthActions()

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)

  if (!config?.moa_security_mode || !moaUser) {
    return null
  }

  const label = moaUser.nick || moaUser.name || moaUser.uid

  const handleLogout = () => {
    removeMoaToken()
    resetMOAAuthentication()
    // Reload so the MOA guard re-evaluates: with the token gone it redirects to
    // the MOA login page.
    window.location.reload()
  }

  return (
    <>
      <IconButton size="small" onClick={(e) => setAnchorEl(e.currentTarget)}>
        <Avatar sx={{ width: 32, height: 32, fontSize: 16 }}>{label.charAt(0).toUpperCase()}</Avatar>
      </IconButton>
      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => setAnchorEl(null)}>
        <Box sx={{ px: 2, py: 1 }}>
          <Typography variant="body2">{label}</Typography>
          <Typography variant="caption" color="text.secondary">
            {moaUser.uid}
          </Typography>
        </Box>
        <Divider />
        <MenuItem onClick={handleLogout}>
          <ListItemIcon>
            <LogoutIcon fontSize="small" />
          </ListItemIcon>
          Logout
        </MenuItem>
      </Menu>
    </>
  )
}

export default MoaUser
