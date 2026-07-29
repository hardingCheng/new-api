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
import { api } from '@/lib/api'

import type { WorkbenchSummaryResponse } from './types'

// 对话采集查看页是服务端直出的，拿不到前端内存里的访问令牌，
// 所以先换一张一分钟有效的一次性入场票，再带票跳过去。
export async function getChatDumpViewerUrl() {
  const res = await api.get<{ success: boolean; data?: { url?: string } }>(
    '/api/chatdump/viewer_ticket'
  )
  return res.data?.data?.url ?? ''
}

export async function getWorkbenchSummary() {
  const res = await api.get<WorkbenchSummaryResponse>('/api/workbench/summary')
  return res.data
}
