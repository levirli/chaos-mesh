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
import { type MoaUser, createMoaStorage } from '@/lib/moaStorage'

export interface MoaCallbackParams {
  token: string
  user: MoaUser
}

export function parseMoaCallback(search: string): MoaCallbackParams | null {
  const params = new URLSearchParams(search)
  const uid = params.get('uid')
  const nick = params.get('nick')
  const name = params.get('name')
  const token = params.get('token')

  if (!uid || !nick || !name || !token) {
    return null
  }

  return {
    token,
    user: { uid, nick, name },
  }
}

export function buildMoaLoginUrl(loginUrl: string, projectId: string, redirect: string): string {
  const url = new URL(loginUrl)
  url.searchParams.set('project', projectId)
  url.searchParams.set('redirect', redirect)
  return url.toString()
}

/**
 * Consume the MOA callback (if any) from the URL search string:
 * persist token/user to storage, strip the query params from the URL
 * (keeping the hash route), and return whether a callback was consumed.
 */
export function consumeMoaCallback(search: string): boolean {
  // MOA may return the callback params in the query string (…/?token=…#/route)
  // or, because the app uses a hash router, after the fragment
  // (…/#/route?token=…). Try the search first, then fall back to the hash's
  // own query so we don't loop back to the login page.
  let callback = parseMoaCallback(search)
  if (!callback) {
    const hashQueryIndex = window.location.hash.indexOf('?')
    if (hashQueryIndex >= 0) {
      callback = parseMoaCallback(window.location.hash.slice(hashQueryIndex))
    }
  }

  if (!callback) {
    return false
  }

  const storage = createMoaStorage()
  storage.setToken(callback.token)
  storage.setUser(callback.user)

  // Drop the callback params from both the search and the hash query, keeping
  // the hash router path so the reload lands on the right route.
  const cleanHash = window.location.hash.split('?')[0]
  window.history.replaceState({}, document.title, window.location.pathname + cleanHash)

  return true
}

export type MoaAuthResult =
  | { status: 'authenticated'; token: string; header: string; user: MoaUser | null }
  | { status: 'redirect'; loginUrl: string }

/**
 * Resolve the current MOA auth state from storage. Pure (no interceptor side
 * effects): the caller is responsible for applying the token to the HTTP
 * client when `status === 'authenticated'`.
 */
export function ensureMoaAuth(config: {
  moa_login_url?: string
  moa_project_id?: string
  moa_token_header?: string
}): MoaAuthResult {
  const storage = createMoaStorage()
  const token = storage.getToken()

  if (token) {
    return {
      status: 'authenticated',
      token,
      header: config.moa_token_header || 'Moa-Token',
      user: storage.getUser(),
    }
  }

  return {
    status: 'redirect',
    loginUrl: buildMoaLoginUrl(
      config.moa_login_url || 'https://login.moa.moonton.net/login',
      config.moa_project_id || '',
      window.location.href,
    ),
  }
}
