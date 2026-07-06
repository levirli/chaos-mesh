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
import {
  applyAPIAuthentication,
  applyErrorHandling,
  applyMOAAuthentication,
  applyNSParam,
  resetMOAAuthentication,
} from '@/api/interceptors'
import { Stale } from '@/api/queryUtils'
import ConfirmDialog from '@/mui-extends/ConfirmDialog'
import Loading from '@/mui-extends/Loading'
import { useGetCommonConfig } from '@/openapi'
import { useAuthActions, useAuthStore } from '@/zustand/auth'
import { useComponentActions, useComponentStore } from '@/zustand/component'
import {
  Alert,
  Box,
  BoxProps,
  Container,
  CssBaseline,
  Divider,
  Portal,
  Snackbar,
  useMediaQuery,
  useTheme,
} from '@mui/material'
import { styled } from '@mui/material/styles'
import Cookies from 'js-cookie'
import { lazy, useEffect, useState } from 'react'
import { Outlet, useNavigate } from 'react-router'

import { TokenFormValues } from '@/components/Token'

import insertCommonStyle from '@/lib/d3/insertCommonStyle'
import LS from '@/lib/localStorage'
import { consumeMoaCallback, ensureMoaAuth } from '@/lib/moaAuth'

import Navbar from './Navbar'
import { closedWidth, openedWidth } from './Sidebar'
import Sidebar from './Sidebar'

const Auth = lazy(() => import('./Auth'))

const Root = styled(Box, {
  shouldForwardProp: (prop) => prop !== 'open',
})<BoxProps & { open: boolean }>(({ theme, open }) => ({
  position: 'relative',
  width: `calc(100% - ${open ? openedWidth : closedWidth}px)`,
  height: '100vh',
  marginLeft: open ? openedWidth : closedWidth,
  transition: theme.transitions.create(['width', 'margin'], {
    easing: theme.transitions.easing.sharp,
    duration: theme.transitions.duration[open ? 'enteringScreen' : 'leavingScreen'],
  }),
  [theme.breakpoints.down('sm')]: {
    minWidth: theme.breakpoints.values.md,
  },
}))

const TopContainer = () => {
  const theme = useTheme()
  const navigate = useNavigate()

  const alert = useComponentStore((state) => state.alert)
  const alertOpen = useComponentStore((state) => state.alertOpen)
  const confirm = useComponentStore((state) => state.confirm)
  const confirmOpen = useComponentStore((state) => state.confirmOpen)
  const { setAlert, setAlertOpen, setConfirmOpen } = useComponentActions()
  const authOpen = useAuthStore((state) => state.authOpen)
  const { setAuthOpen, setNameSpace, setTokenName, setTokens, removeToken, removeMoaToken, setMoaUser } =
    useAuthActions()

  // Sidebar related
  const miniSidebar = LS.get('mini-sidebar') === 'y'
  const [openDrawer, setOpenDrawer] = useState(!miniSidebar)
  const handleDrawerToggle = () => {
    setOpenDrawer(!openDrawer)
    LS.set('mini-sidebar', openDrawer ? 'y' : 'n')
  }

  const [loading, setLoading] = useState(true)

  const { data } = useGetCommonConfig({
    query: {
      staleTime: Stale.DAY,
    },
  })

  // MOA guard — runs above the RBAC auth dialog. In MOA mode the dialog is
  // never used: we either restore the persisted token, or redirect to the MOA
  // login page automatically.
  useEffect(() => {
    if (!data || !data.moa_security_mode) {
      return
    }

    // Returning from MOA login: persist the callback token, then reload so the
    // guard re-evaluates with the token in storage.
    if (consumeMoaCallback(window.location.search)) {
      navigate(0)

      return
    }

    const result = ensureMoaAuth(data)
    if (result.status === 'authenticated') {
      applyMOAAuthentication({ token: result.token, header: result.header })
      setMoaUser(result.user)
      setLoading(false)
    } else {
      // No token yet — bounce to MOA login. Keep `loading` true so the RBAC
      // dialog never flashes before the navigation fires.
      window.location.href = result.loginUrl
    }
  }, [data, navigate, setMoaUser])

  useEffect(() => {
    /**
     * Set authorization (RBAC token / GCP) for API use. MOA is handled
     * exclusively by the guard effect above and never reaches here.
     */
    function setAuth() {
      // GCP
      const accessToken = Cookies.get('access_token')
      const expiry = Cookies.get('expiry')

      if (accessToken && expiry) {
        const token = {
          accessToken,
          expiry,
        }

        applyAPIAuthentication(token)
        setTokenName('gcp')

        return
      }

      const token = LS.get('token')
      const tokenName = LS.get('token-name')
      const globalNamespace = LS.get('global-namespace')

      if (token && tokenName) {
        const tokens: TokenFormValues[] = JSON.parse(token)

        applyAPIAuthentication(tokens.find(({ name }) => name === tokenName)!.token)
        setTokens(tokens)
        setTokenName(tokenName)
      } else {
        setAuthOpen(true)
      }

      if (globalNamespace) {
        applyNSParam(globalNamespace)
        setNameSpace(globalNamespace)
      }
    }

    if (!data) {
      return
    }

    // MOA mode is owned by the guard effect; never fall through to RBAC.
    if (data.security_mode && !data.moa_security_mode) {
      setAuth()
    }

    // Lower loading only outside MOA mode — in MOA mode the guard effect is
    // responsible (on `authenticated`), or stays true during redirect.
    if (!data.moa_security_mode) {
      setLoading(false)
    }
  }, [data, setAuthOpen, setNameSpace, setTokenName, setTokens])

  useEffect(() => {
    applyErrorHandling({
      openAlert: setAlert,
      removeToken: () => {
        removeToken()
        removeMoaToken()
        resetMOAAuthentication()
      },
    })
    insertCommonStyle()
  }, [data, removeMoaToken, removeToken, setAlert])

  const isTabletScreen = useMediaQuery(theme.breakpoints.down('md'))
  useEffect(() => {
    if (isTabletScreen) {
      setOpenDrawer(false)
    }
  }, [isTabletScreen])

  return (
    <>
      <CssBaseline />
      <Root open={openDrawer}>
        <Sidebar open={openDrawer} />
        <Box component="main" sx={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
          <Navbar openDrawer={openDrawer} handleDrawerToggle={handleDrawerToggle} />
          <Divider />

          <Container maxWidth="xl" disableGutters sx={{ flexGrow: 1, p: 6 }}>
            {loading ? <Loading /> : <Outlet />}
          </Container>
        </Box>
      </Root>

      <Auth open={authOpen} />

      <Portal>
        <Snackbar
          anchorOrigin={{
            vertical: 'bottom',
            horizontal: 'center',
          }}
          autoHideDuration={6000}
          open={alertOpen}
          onClose={() => setAlertOpen(false)}
        >
          <Alert severity={alert.type} onClose={() => setAlertOpen(false)}>
            {alert.message}
          </Alert>
        </Snackbar>
      </Portal>

      <Portal>
        <ConfirmDialog
          open={confirmOpen}
          close={() => setConfirmOpen(false)}
          title={confirm.title}
          description={confirm.description}
          onConfirm={confirm.handle}
        />
      </Portal>
    </>
  )
}

export default TopContainer
