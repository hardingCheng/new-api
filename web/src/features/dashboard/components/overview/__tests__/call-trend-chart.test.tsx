/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { OverviewDashboard } from '../prism-overview'

const DAY = 86400
const NOW = 1790000000
const START = NOW - 7 * DAY

const getUserQuotaDates = vi.hoisted(() => vi.fn())
vi.mock('@/features/dashboard/api', () => ({ getUserQuotaDates }))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => children,
}))

vi.mock('@/features/dashboard/components/overview/announcements-panel', () => ({
  AnnouncementsPanel: () => null,
}))
vi.mock(
  '@/features/dashboard/components/overview/performance-health-panel',
  () => ({ PerformanceHealthPanel: () => null })
)

let client: QueryClient

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(NOW * 1000)
  getUserQuotaDates.mockResolvedValue({
    success: true,
    data: [{ created_at: START + DAY * 2, count: 1200, token_used: 900, quota: 1500 }],
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
})

afterEach(() => {
  client.clear()
  vi.useRealTimers()
})

function renderDashboard() {
  return render(
    <QueryClientProvider client={client}>
      <OverviewDashboard />
    </QueryClientProvider>
  )
}

it('reserves a measurable track for each trend bar so percentage heights resolve', async () => {
  renderDashboard()

  const bars = await screen.findAllByTestId('call-trend-bar')
  expect(bars).toHaveLength(8)

  for (const bar of bars) {
    const track = bar.parentElement
    expect(track).not.toBeNull()
    // The bar's height is a percentage, so its containing block must have a
    // definite height; otherwise the browser resolves every bar to 0px and the
    // chart renders as an empty box with only the date axis.
    expect(track?.className).toContain('h-32')
    expect(track?.className).toContain('items-end')
  }
})

it('keeps a visible minimum height for days with no usage', async () => {
  renderDashboard()

  const bars = await screen.findAllByTestId('call-trend-bar')
  const heights = bars.map((bar) => bar.getAttribute('style') ?? '')

  expect(heights.every((style) => style.includes('height:'))).toBe(true)
  expect(heights).toContain('height: 2%;')
})
