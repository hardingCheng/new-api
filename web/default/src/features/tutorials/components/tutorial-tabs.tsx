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
import { ExternalLink, Info } from 'lucide-react'

import { Button } from '@/components/design-system/button'
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/design-system/tabs'
import { Alert, AlertDescription } from '@/components/ui/alert'

import type { TutorialTabGroup } from '../tutorial-content'
import { TutorialCodeBlock } from './tutorial-code-block'

type TutorialTabsProps = {
  group: TutorialTabGroup
  depth?: number
}

export function TutorialTabs({ group, depth = 0 }: TutorialTabsProps) {
  return (
    <div className='space-y-2'>
      {group.label ? (
        <p className='text-muted-foreground text-xs font-medium'>
          {group.label}
        </p>
      ) : null}
      <Tabs defaultValue={group.defaultValue} className='gap-3'>
        <div className='max-w-full overflow-x-auto pb-1'>
          <TabsList
            aria-label={group.ariaLabel}
            className='h-auto min-w-max justify-start'
            variant={depth > 0 ? 'line' : 'default'}
          >
            {group.tabs.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value}>
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>

        {group.tabs.map((tab) => (
          <TabsContent key={tab.value} value={tab.value} className='mt-0'>
            <div className='bg-muted/25 space-y-3 rounded-[var(--radius-lg)] border p-3.5 sm:p-4'>
              {tab.title || tab.description ? (
                <div className='space-y-1'>
                  {tab.title ? (
                    <h4 className='text-sm font-semibold'>{tab.title}</h4>
                  ) : null}
                  {tab.description ? (
                    <p className='text-muted-foreground text-sm leading-6'>
                      {tab.description}
                    </p>
                  ) : null}
                </div>
              ) : null}

              {tab.codeBlocks?.map((block) => (
                <TutorialCodeBlock
                  key={`${tab.value}-${block.label}`}
                  label={block.label}
                  code={block.code}
                />
              ))}

              {tab.tabGroups?.map((nestedGroup) => (
                <TutorialTabs
                  key={`${tab.value}-${nestedGroup.ariaLabel}`}
                  group={nestedGroup}
                  depth={depth + 1}
                />
              ))}

              {tab.actionUrl && tab.actionLabel ? (
                <Button
                  variant='outline'
                  render={
                    <a href={tab.actionUrl} target='_blank' rel='noreferrer' />
                  }
                >
                  {tab.actionLabel}
                  <ExternalLink data-icon='inline-end' />
                </Button>
              ) : null}

              {tab.note ? (
                <Alert className='bg-background/70'>
                  <Info />
                  <AlertDescription>{tab.note}</AlertDescription>
                </Alert>
              ) : null}
            </div>
          </TabsContent>
        ))}
      </Tabs>
    </div>
  )
}
