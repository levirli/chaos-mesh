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
import { buildMoaLoginUrl, consumeMoaCallback, ensureMoaAuth, parseMoaCallback } from '@/lib/moaAuth'
import { createMoaStorage } from '@/lib/moaStorage'

describe('moaAuth', () => {
  describe('parseMoaCallback', () => {
    it('parses all callback parameters', () => {
      const result = parseMoaCallback('?uid=1001374&nick=levirli&name=test&token=abc')
      expect(result).toEqual({
        token: 'abc',
        user: { uid: '1001374', nick: 'levirli', name: 'test' },
      })
    })

    it('returns null when a parameter is missing', () => {
      expect(parseMoaCallback('?uid=1001374&nick=levirli&name=test')).toBeNull()
      expect(parseMoaCallback('?uid=1001374&nick=levirli&token=abc')).toBeNull()
      expect(parseMoaCallback('')).toBeNull()
    })
  })

  describe('buildMoaLoginUrl', () => {
    it('encodes project and redirect parameters', () => {
      const url = buildMoaLoginUrl('https://login.moa.moonton.net/login', '1600', 'http://localhost/#/dashboard')
      expect(url).toBe(
        'https://login.moa.moonton.net/login?project=1600&redirect=http%3A%2F%2Flocalhost%2F%23%2Fdashboard',
      )
    })
  })

  describe('consumeMoaCallback', () => {
    beforeEach(() => {
      window.localStorage.clear()
    })

    it('persists token and user to localStorage when callback is present', () => {
      const replaceState = jest.spyOn(window.history, 'replaceState').mockImplementation(() => {})

      const consumed = consumeMoaCallback('?uid=1&nick=levi&name=test&token=abc')

      expect(consumed).toBe(true)
      expect(createMoaStorage().getToken()).toBe('abc')
      expect(createMoaStorage().getUser()).toEqual({ uid: '1', nick: 'levi', name: 'test' })
      expect(replaceState).toHaveBeenCalled()

      replaceState.mockRestore()
    })

    it('returns false and does not persist when params are missing', () => {
      const replaceState = jest.spyOn(window.history, 'replaceState').mockImplementation(() => {})

      const consumed = consumeMoaCallback('?uid=1&nick=levi')

      expect(consumed).toBe(false)
      expect(createMoaStorage().getToken()).toBeNull()
      expect(replaceState).not.toHaveBeenCalled()

      replaceState.mockRestore()
    })
  })

  describe('ensureMoaAuth', () => {
    beforeEach(() => {
      window.localStorage.clear()
    })

    it('returns authenticated with token when storage has one', () => {
      createMoaStorage().setToken('stored-token')

      const result = ensureMoaAuth({
        moa_login_url: 'https://login.moa.moonton.net/login',
        moa_project_id: '1600',
        moa_token_header: 'Authorization',
      })

      expect(result.status).toBe('authenticated')
      if (result.status === 'authenticated') {
        expect(result.token).toBe('stored-token')
        expect(result.header).toBe('Authorization')
      }
    })

    it('returns redirect with login url when storage is empty', () => {
      const result = ensureMoaAuth({
        moa_login_url: 'https://login.moa.moonton.net/login',
        moa_project_id: '1600',
        moa_token_header: 'Authorization',
      })

      expect(result.status).toBe('redirect')
      if (result.status === 'redirect') {
        expect(result.loginUrl).toContain('project=1600')
        expect(result.loginUrl).toContain('redirect=')
        expect(result.loginUrl).toContain('https://login.moa.moonton.net/login')
      }
    })

    it('falls back to defaults when config fields are missing', () => {
      const result = ensureMoaAuth({})

      expect(result.status).toBe('redirect')
      if (result.status === 'redirect') {
        expect(result.loginUrl).toContain('https://login.moa.moonton.net/login')
      }
    })
  })
})

describe('moaStorage', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('stores token and user in localStorage', () => {
    const storage = createMoaStorage()
    storage.setToken('token-value')
    storage.setUser({ uid: '1', nick: 'nick', name: 'name' })

    expect(storage.getToken()).toBe('token-value')
    expect(storage.getUser()).toEqual({ uid: '1', nick: 'nick', name: 'name' })

    storage.removeToken()
    storage.removeUser()
    expect(storage.getToken()).toBeNull()
    expect(storage.getUser()).toBeNull()
  })
})
