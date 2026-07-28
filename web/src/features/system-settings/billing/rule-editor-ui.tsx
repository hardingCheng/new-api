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
import type { ReactNode } from 'react'

import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export function EditorField(props: {
  label: ReactNode
  children: ReactNode
  hint?: ReactNode
}) {
  return (
    <div className='grid gap-2'>
      <Label>{props.label}</Label>
      {props.children}
      {props.hint ? (
        <p className='text-muted-foreground text-xs'>{props.hint}</p>
      ) : null}
    </div>
  )
}

export function RuleTable(props: {
  headers: ReactNode[]
  children: ReactNode
  empty: boolean
  emptyText: ReactNode
}) {
  return (
    <div className='overflow-x-auto rounded-lg border'>
      <Table>
        <TableHeader>
          <TableRow>
            {props.headers.map((header) => (
              <TableHead key={String(header)} className='whitespace-nowrap'>
                {header}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.empty ? (
            <TableRow>
              <TableCell
                colSpan={props.headers.length}
                className='text-muted-foreground h-24 text-center'
              >
                {props.emptyText}
              </TableCell>
            </TableRow>
          ) : (
            props.children
          )}
        </TableBody>
      </Table>
    </div>
  )
}
