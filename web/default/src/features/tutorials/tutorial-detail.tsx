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
import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  CheckCircle2,
  Clock3,
  ExternalLink,
  Key,
  ShieldCheck,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/design-system/button'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import { TutorialCodeBlock } from './components/tutorial-code-block'
import { getTutorials } from './tutorial-content'

type TutorialDetailProps = {
  slug: string
}

export function TutorialDetail(props: TutorialDetailProps) {
  const { t } = useTranslation()
  const origin = typeof window === 'undefined' ? '' : window.location.origin
  const tutorials = useMemo(() => getTutorials(t, origin), [origin, t])
  const tutorialIndex = tutorials.findIndex(
    (tutorial) => tutorial.slug === props.slug
  )
  const tutorial = tutorialIndex >= 0 ? tutorials[tutorialIndex] : null

  if (!tutorial) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Tutorial not found')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='flex min-h-80 flex-col items-center justify-center gap-4 text-center'>
            <BookOpen className='text-muted-foreground size-10' />
            <p className='text-muted-foreground text-sm'>
              {t('The requested tutorial does not exist or has been removed.')}
            </p>
            <Button variant='outline' render={<Link to='/tutorials' />}>
              {t('Return to tutorial center')}
            </Button>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }

  const Icon = tutorial.icon
  const previous = tutorialIndex > 0 ? tutorials[tutorialIndex - 1] : null
  const next =
    tutorialIndex < tutorials.length - 1 ? tutorials[tutorialIndex + 1] : null

  return (
    <SectionPageLayout>
      <SectionPageLayout.Breadcrumb>
        <Link
          to='/tutorials'
          className='text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-xs font-medium transition-colors'
        >
          <ArrowLeft className='size-3.5' />
          {t('Tutorial Center')}
        </Link>
      </SectionPageLayout.Breadcrumb>
      <SectionPageLayout.Title>{tutorial.title}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' render={<Link to='/keys' />}>
          <Key data-icon='inline-start' />
          {t('Open API Keys')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='mx-auto max-w-6xl space-y-4 pb-8'>
          <section className='bg-card ring-foreground/10 rounded-2xl p-5 ring-1 sm:p-7'>
            <div className='flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between'>
              <div className='flex min-w-0 gap-4'>
                <div
                  className={`flex size-12 shrink-0 items-center justify-center rounded-2xl ${tutorial.accentClassName}`}
                >
                  <Icon className='size-6' />
                </div>
                <div className='min-w-0 space-y-2'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <Badge variant='secondary'>{tutorial.category}</Badge>
                    <span className='text-muted-foreground inline-flex items-center gap-1 text-xs'>
                      <Clock3 className='size-3.5' />
                      {tutorial.duration}
                    </span>
                    <span className='text-muted-foreground inline-flex items-center gap-1 text-xs'>
                      <ShieldCheck className='size-3.5' />
                      {tutorial.level}
                    </span>
                  </div>
                  <p className='text-muted-foreground max-w-2xl text-sm leading-6'>
                    {tutorial.description}
                  </p>
                </div>
              </div>
              {tutorial.officialUrl ? (
                <Button
                  variant='outline'
                  className='self-start'
                  render={
                    <a
                      href={tutorial.officialUrl}
                      target='_blank'
                      rel='noreferrer'
                    />
                  }
                >
                  {t('Official documentation')}
                  <ExternalLink data-icon='inline-end' />
                </Button>
              ) : null}
            </div>
          </section>

          <div className='grid gap-4 lg:grid-cols-[210px_minmax(0,1fr)]'>
            <aside className='hidden lg:block'>
              <nav className='bg-card sticky top-4 rounded-xl border p-3'>
                <p className='text-muted-foreground px-2 pb-2 text-xs font-medium tracking-wider uppercase'>
                  {t('In this guide')}
                </p>
                <div className='space-y-1'>
                  {tutorial.steps.map((step, index) => (
                    <a
                      key={step.title}
                      href={`#step-${index + 1}`}
                      className='hover:bg-muted flex items-start gap-2 rounded-lg px-2 py-2 text-xs leading-5 transition-colors'
                    >
                      <span className='bg-muted text-muted-foreground mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-md font-mono text-[10px]'>
                        {String(index + 1).padStart(2, '0')}
                      </span>
                      <span>{step.title}</span>
                    </a>
                  ))}
                </div>
              </nav>
            </aside>

            <div className='space-y-4'>
              {tutorial.steps.map((step, index) => (
                <Card
                  key={step.title}
                  id={`step-${index + 1}`}
                  className='scroll-mt-4'
                >
                  <CardHeader>
                    <div className='flex items-start gap-3'>
                      <div className='bg-primary text-primary-foreground flex size-8 shrink-0 items-center justify-center rounded-lg font-mono text-xs font-semibold'>
                        {String(index + 1).padStart(2, '0')}
                      </div>
                      <div className='min-w-0 space-y-1'>
                        <CardTitle>{step.title}</CardTitle>
                        <CardDescription className='leading-6'>
                          {step.description}
                        </CardDescription>
                      </div>
                    </div>
                  </CardHeader>
                  {step.codeBlocks || step.note ? (
                    <CardContent className='space-y-3 pl-4 sm:pl-15'>
                      {step.codeBlocks?.map((block) => (
                        <TutorialCodeBlock
                          key={`${step.title}-${block.label}`}
                          label={block.label}
                          code={block.code}
                        />
                      ))}
                      {step.note ? (
                        <Alert className='bg-muted/35'>
                          <CheckCircle2 />
                          <AlertDescription>{step.note}</AlertDescription>
                        </Alert>
                      ) : null}
                    </CardContent>
                  ) : null}
                </Card>
              ))}

              <div className='grid gap-3 pt-2 sm:grid-cols-2'>
                {previous ? (
                  <Link
                    to='/tutorials/$slug'
                    params={{ slug: previous.slug }}
                    className='hover:bg-muted bg-card rounded-xl border p-4 transition-colors'
                  >
                    <span className='text-muted-foreground flex items-center gap-1 text-xs'>
                      <ArrowLeft className='size-3.5' />
                      {t('Previous')}
                    </span>
                    <span className='mt-1 block text-sm font-medium'>
                      {previous.title}
                    </span>
                  </Link>
                ) : (
                  <div />
                )}
                {next ? (
                  <Link
                    to='/tutorials/$slug'
                    params={{ slug: next.slug }}
                    className='hover:bg-muted bg-card rounded-xl border p-4 text-right transition-colors'
                  >
                    <span className='text-muted-foreground flex items-center justify-end gap-1 text-xs'>
                      {t('Next')}
                      <ArrowRight className='size-3.5' />
                    </span>
                    <span className='mt-1 block text-sm font-medium'>
                      {next.title}
                    </span>
                  </Link>
                ) : null}
              </div>
            </div>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
