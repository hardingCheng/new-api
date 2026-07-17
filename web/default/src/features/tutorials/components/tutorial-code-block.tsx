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
import { CopyButton } from '@/components/copy-button'

type TutorialCodeBlockProps = {
  label: string
  code: string
}

export function TutorialCodeBlock(props: TutorialCodeBlockProps) {
  return (
    <div className='ring-foreground/10 overflow-hidden rounded-xl bg-[var(--tutorial-code)] text-[var(--tutorial-code-foreground)] ring-1'>
      <div className='flex items-center justify-between gap-3 border-b border-[color-mix(in_oklch,var(--tutorial-code-foreground)_14%,transparent)] px-3 py-2'>
        <span className='truncate font-mono text-[11px] font-medium tracking-wider uppercase opacity-70'>
          {props.label}
        </span>
        <CopyButton
          value={props.code}
          size='icon-xs'
          className='text-[var(--tutorial-code-foreground)] hover:bg-[color-mix(in_oklch,var(--tutorial-code-foreground)_12%,transparent)] hover:text-[var(--tutorial-code-foreground)]'
        />
      </div>
      <pre className='overflow-x-auto px-4 py-3 text-xs leading-6 sm:text-[13px]'>
        <code>{props.code}</code>
      </pre>
    </div>
  )
}
