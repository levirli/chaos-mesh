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

const MOA_TOKEN_KEY = 'moa-token'
const MOA_USER_KEY = 'moa-user'

export interface MoaUser {
  uid: string
  nick: string
  name: string
}

export interface MoaStorage {
  getToken: () => string | null
  setToken: (token: string) => void
  removeToken: () => void
  getUser: () => MoaUser | null
  setUser: (user: MoaUser) => void
  removeUser: () => void
}

export function createMoaStorage(): MoaStorage {
  return {
    getToken: () => window.localStorage.getItem(MOA_TOKEN_KEY),
    setToken: (token: string) => window.localStorage.setItem(MOA_TOKEN_KEY, token),
    removeToken: () => window.localStorage.removeItem(MOA_TOKEN_KEY),
    getUser: () => {
      const raw = window.localStorage.getItem(MOA_USER_KEY)
      if (!raw) {
        return null
      }
      try {
        return JSON.parse(raw) as MoaUser
      } catch {
        return null
      }
    },
    setUser: (user: MoaUser) => window.localStorage.setItem(MOA_USER_KEY, JSON.stringify(user)),
    removeUser: () => window.localStorage.removeItem(MOA_USER_KEY),
  }
}
