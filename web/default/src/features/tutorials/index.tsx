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
import { Link } from '@tanstack/react-router'
import { ArrowRight, Clock3, Key, Layers3 } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/design-system/button'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import { getTutorials } from './tutorial-content'

export function Tutorials() {
  const { t } = useTranslation()
  const origin = typeof window === 'undefined' ? '' : window.location.origin
  const tutorials = useMemo(() => getTutorials(t, origin), [origin, t])
  const apiBaseUrl = `${origin}/v1`

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Tutorial Center')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto max-w-6xl space-y-4 pb-8'>
          <section className='bg-primary text-primary-foreground overflow-hidden rounded-2xl px-5 py-6 shadow-sm sm:px-8 sm:py-8'>
            <div className='grid gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(300px,0.6fr)] lg:items-end'>
              <div className='max-w-2xl space-y-4'>
                <Badge className='bg-primary-foreground/12 text-primary-foreground border-primary-foreground/15'>
                  {t('Start here')}
                </Badge>
                <div className='space-y-2'>
                  <h1 className='text-2xl font-semibold tracking-tight sm:text-3xl'>
                    {t('Connect your favorite AI tools')}
                  </h1>
                  <p className='text-primary-foreground/75 max-w-xl text-sm leading-6 sm:text-base'>
                    {t(
                      'Use the current station address in Codex, Claude Code, Gemini CLI, CC Switch, or any OpenAI-compatible client.'
                    )}
                  </p>
                </div>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    variant='secondary'
                    render={
                      <Link
                        to='/tutorials/$slug'
                        params={{ slug: 'quick-start' }}
                      />
                    }
                  >
                    {t('View quick start')}
                    <ArrowRight data-icon='inline-end' />
                  </Button>
                  <Button
                    variant='outline'
                    className='border-primary-foreground/25 text-primary-foreground hover:bg-primary-foreground/10 hover:text-primary-foreground bg-transparent'
                    render={<Link to='/keys' />}
                  >
                    <Key data-icon='inline-start' />
                    {t('Open API Keys')}
                  </Button>
                </div>
              </div>

              <div className='bg-primary-foreground/8 border-primary-foreground/15 rounded-xl border p-3'>
                <div className='mb-2 flex items-center justify-between gap-3'>
                  <span className='text-primary-foreground/65 text-xs font-medium'>
                    Base URL
                  </span>
                  <CopyButton
                    value={apiBaseUrl}
                    size='icon-xs'
                    className='text-primary-foreground hover:bg-primary-foreground/10 hover:text-primary-foreground'
                  />
                </div>
                <code className='block overflow-x-auto font-mono text-sm whitespace-nowrap'>
                  {apiBaseUrl}
                </code>
              </div>
            </div>
          </section>

          <div className='flex items-end justify-between gap-4 pt-3'>
            <div>
              <h2 className='text-lg font-semibold'>
                {t('Choose a tutorial')}
              </h2>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t(
                  'Each guide uses the current station address automatically.'
                )}
              </p>
            </div>
            <div className='text-muted-foreground hidden items-center gap-1.5 text-xs sm:flex'>
              <Layers3 className='size-3.5' />
              {t('{{count}} guides', { count: tutorials.length })}
            </div>
          </div>

          <section className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
            {tutorials.map((tutorial) => {
              const Icon = tutorial.icon
              return (
                <Link
                  key={tutorial.slug}
                  to='/tutorials/$slug'
                  params={{ slug: tutorial.slug }}
                  className='group focus-visible:ring-ring/50 rounded-xl outline-none focus-visible:ring-3'
                >
                  <Card className='h-full transition-[transform,box-shadow] duration-200 group-hover:-translate-y-0.5 group-hover:shadow-md'>
                    <CardHeader>
                      <div
                        className={`mb-2 flex size-10 items-center justify-center rounded-xl ${tutorial.accentClassName}`}
                      >
                        <Icon className='size-5' />
                      </div>
                      <CardTitle className='flex items-center justify-between gap-3'>
                        <span>{tutorial.title}</span>
                        <ArrowRight className='text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5' />
                      </CardTitle>
                      <CardDescription className='line-clamp-2 min-h-10 leading-5'>
                        {tutorial.description}
                      </CardDescription>
                    </CardHeader>
                    <CardContent className='mt-auto flex items-center justify-between gap-3'>
                      <Badge variant='secondary'>{tutorial.category}</Badge>
                      <span className='text-muted-foreground inline-flex items-center gap-1 text-xs'>
                        <Clock3 className='size-3.5' />
                        {tutorial.duration}
                      </span>
                    </CardContent>
                  </Card>
                </Link>
              )
            })}
          </section>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

export { TutorialDetail } from './tutorial-detail'
