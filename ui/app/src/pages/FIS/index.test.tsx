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
import { render, screen } from '@/test-utils'

import FIS from './index'

// Mock the FIS API client so the New page does not post a CR.
const mockCreateFIStemplate = jest.fn()

jest.mock('@/api/fis', () => ({
  createFIStemplate: function () {
    return mockCreateFIStemplate.apply(null, arguments)
  },
}))

// Mock http (used for RDS listing endpoints) so it doesn't hit the network.
jest.mock('@/api/http', () => ({
  default: { get: jest.fn().mockResolvedValue({ data: [] }) },
}))

// react-router useNavigate is used on submit success; stub it.
const mockNavigate = jest.fn()
jest.mock('react-router', () => {
  const actual = jest.requireActual('react-router')
  return { ...actual, useNavigate: () => mockNavigate }
})

describe('FIS New Experiment Template page', () => {
  afterEach(() => {
    jest.clearAllMocks()
  })

  it('renders without crashing', () => {
    render(<FIS />)
    // The page title is "New FIS Experiment Template".
    expect(screen.getByText('New FIS Experiment Template')).toBeInTheDocument()
  })
})
