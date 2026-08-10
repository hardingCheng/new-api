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
import { getRouteApi } from '@tanstack/react-router'
import { Download, Loader2 } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { getTaskExport } from '../api'
import {
  buildTaskExportParams,
  buildTaskExportRanges,
  buildTaskExportRows,
  getTaskExportErrorKey,
  getTaskExportValidationError,
  TASK_EXPORT_PAGE_SIZE,
} from '../lib/task-export'

const route = getRouteApi('/_authenticated/usage-logs/$section')

export function TaskLogsExportButton() {
  const { t } = useTranslation()
  const searchParams = route.useSearch()
  const [isExporting, setIsExporting] = useState(false)

  const preloadSpreadsheetLibrary = useCallback(() => {
    void import('xlsx')
  }, [])

  const handleExport = useCallback(async () => {
    const exportParams = buildTaskExportParams(searchParams)
    const validationError = getTaskExportValidationError(exportParams)
    if (validationError) {
      toast.warning(t(validationError))
      return
    }

    setIsExporting(true)
    const progressToastId = toast.loading(t('Processing...'))
    try {
      const spreadsheetPromise = import('xlsx')
      let worksheet: import('xlsx').WorkSheet | undefined
      let headings: string[] = []
      for (const range of buildTaskExportRanges(exportParams)) {
        let beforeID: string | undefined
        const seenCursors = new Set<string>()
        while (true) {
          const result = await getTaskExport({
            ...range,
            limit: TASK_EXPORT_PAGE_SIZE,
            ...(beforeID ? { before_id: beforeID } : {}),
          })
          if (!result.success) {
            toast.error(t(getTaskExportErrorKey(result.message)), {
              id: progressToastId,
            })
            return
          }

          const pageRows = buildTaskExportRows(result.data?.items ?? [], t)
          if (pageRows.length > 0) {
            const spreadsheet = await spreadsheetPromise
            if (worksheet) {
              spreadsheet.utils.sheet_add_json(worksheet, pageRows, {
                skipHeader: true,
                origin: -1,
              })
            } else {
              headings = Object.keys(pageRows[0])
              worksheet = spreadsheet.utils.json_to_sheet(pageRows)
            }
          }
          if (!result.data?.has_more) break

          const nextCursor = result.data.next_cursor
          if (!nextCursor || seenCursors.has(nextCursor)) {
            throw new Error('invalid task export cursor')
          }
          seenCursors.add(nextCursor)
          beforeID = nextCursor
        }
      }

      if (!worksheet) {
        toast.info(t('No data to export'), { id: progressToastId })
        return
      }

      const spreadsheet = await spreadsheetPromise
      worksheet['!cols'] = headings.map((heading) => ({
        wch: Math.min(Math.max(heading.length + 4, 14), 32),
      }))
      const workbook = spreadsheet.utils.book_new()
      spreadsheet.utils.book_append_sheet(workbook, worksheet, t('Task Logs'))
      const timestamp = new Date()
        .toISOString()
        .replaceAll(/[-:T.Z]/g, '')
        .slice(0, 14)
      spreadsheet.writeFile(workbook, `task-report-${timestamp}.xlsx`)
      toast.success(t('Export successful'), { id: progressToastId })
    } catch {
      toast.error(t('Export failed. Please try again.'), {
        id: progressToastId,
      })
    } finally {
      setIsExporting(false)
    }
  }, [searchParams, t])

  const button = (
    <Button
      type='button'
      variant='outline'
      size='icon'
      aria-label={t('Export Report')}
      disabled={isExporting}
      onMouseEnter={preloadSpreadsheetLibrary}
      onFocus={preloadSpreadsheetLibrary}
      onClick={() => void handleExport()}
    >
      {isExporting ? (
        <Loader2 className='animate-spin' aria-hidden='true' />
      ) : (
        <Download aria-hidden='true' />
      )}
    </Button>
  )

  return (
    <Tooltip>
      <TooltipTrigger render={button} />
      <TooltipContent>{t('Export Report')}</TooltipContent>
    </Tooltip>
  )
}
