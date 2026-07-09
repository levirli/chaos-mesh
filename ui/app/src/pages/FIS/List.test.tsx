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
import { render, screen, waitFor } from '@/test-utils'

import FISList from './List'

// Mock the FIS API client so the List page does not hit the network.
const mockListFIStemplates = jest.fn()
const mockDeleteFIStemplate = jest.fn()
const mockStartFISexperiment = jest.fn()

jest.mock('@/api/fis', () => ({
  listFIStemplates: function () {
    return mockListFIStemplates.apply(null, arguments)
  },
  deleteFIStemplate: function () {
    return mockDeleteFIStemplate.apply(null, arguments)
  },
  startFISexperiment: function () {
    return mockStartFISexperiment.apply(null, arguments)
  },
  listFISexperiments: jest.fn(),
}))

describe('FISList', () => {
  afterEach(() => {
    jest.clearAllMocks()
  })

  it('renders the top buttons', async () => {
    mockListFIStemplates.mockResolvedValue([])
    render(<FISList />)

    expect(await screen.findByText('New Experiment Template')).toBeInTheDocument()
    expect(screen.getByText('Experiment Info')).toBeInTheDocument()
  })

  it('renders templates from the API', async () => {
    mockListFIStemplates.mockResolvedValue([
      {
        id: 'EXT123',
        description: 'reboot mydb',
        creationTime: '2026-07-07T10:00:00Z',
        tags: { Name: 'chaos-mesh/abc' },
      },
    ])

    render(<FISList />)

    await waitFor(() => {
      expect(screen.getByText('EXT123')).toBeInTheDocument()
    })
    expect(screen.getByText('reboot mydb')).toBeInTheDocument()
  })
})
