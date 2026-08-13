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
  Activity,
  CircleDollarSign,
  Route,
  Users,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import type { WorkbenchStatusBar } from '../types'

/** 入口的去处。存路由 + section 而不是拼好的路径:这样跳转由 TanStack Router
 *  校验并在 SPA 内完成,不像裸 <a href> 那样每点一次都重载整个后台。 */
type AreaTarget =
  | { to: '/channels' }
  | { to: '/users' }
  | { to: '/system-info' }
  | { to: '/models/$section'; section: 'metadata' }
  | { to: '/usage-logs/$section'; section: 'common' }
  | {
      to: '/system-settings/billing/$section'
      section: 'model-pricing' | 'group-pricing' | 'user-pricing-override'
    }
  | {
      to: '/system-settings/operations/$section'
      section: 'channel-breaker' | 'performance'
    }
  | { to: '/system-settings/security/$section'; section: 'rate-limit' }

/** 对话采集要先向后端换一个一次性地址,所以它是按钮而不是链接。 */
type AreaLink =
  | { labelKey: string; target: AreaTarget }
  | { labelKey: string; action: 'captures' }

interface ManagementArea {
  titleKey: string
  icon: LucideIcon
  links: AreaLink[]
  /** null = 算不出来(数据源挂了),此时不显示徽章而不是显示一个偏低的数。 */
  signalCount?: number | null
}

/** 路由字面量和它的 params 必须写在一起,类型检查才能确认两者匹配。 */
function areaTargetLink(target: AreaTarget) {
  switch (target.to) {
    case '/channels':
      return <Link to='/channels' />
    case '/users':
      return <Link to='/users' />
    case '/system-info':
      return <Link to='/system-info' />
    case '/models/$section':
      return <Link to='/models/$section' params={{ section: target.section }} />
    case '/usage-logs/$section':
      return (
        <Link to='/usage-logs/$section' params={{ section: target.section }} />
      )
    case '/system-settings/billing/$section':
      return (
        <Link
          to='/system-settings/billing/$section'
          params={{ section: target.section }}
        />
      )
    case '/system-settings/operations/$section':
      return (
        <Link
          to='/system-settings/operations/$section'
          params={{ section: target.section }}
        />
      )
    case '/system-settings/security/$section':
      return (
        <Link
          to='/system-settings/security/$section'
          params={{ section: target.section }}
        />
      )
  }
}

function ManagementAreaRow({
  area,
  onOpenCaptures,
  openingCaptures,
}: {
  area: ManagementArea
  onOpenCaptures: () => Promise<void>
  openingCaptures: boolean
}) {
  const { t } = useTranslation()
  const Icon = area.icon

  return (
    <section className='bg-card p-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Icon className='text-muted-foreground size-4' aria-hidden='true' />
        <h3 className='font-medium'>{t(area.titleKey)}</h3>
        {area.signalCount != null && area.signalCount > 0 && (
          <StatusBadge variant='warning' className='ms-auto'>
            {t('{{count}} signals', { count: area.signalCount })}
          </StatusBadge>
        )}
      </div>
      <div className='mt-2 flex flex-wrap gap-1.5'>
        {area.links.map((link) => {
          if ('action' in link) {
            return (
              <Button
                key={link.labelKey}
                variant='outline'
                size='xs'
                disabled={openingCaptures}
                onClick={() => void onOpenCaptures()}
              >
                {t(link.labelKey)}
              </Button>
            )
          }

          return (
            <Button
              key={link.labelKey}
              variant='outline'
              size='xs'
              render={areaTargetLink(link.target)}
            >
              {t(link.labelKey)}
            </Button>
          )
        })}
      </div>
    </section>
  )
}

export function ManagementAreas({
  statusBar,
  onOpenCaptures,
  openingCaptures,
}: {
  statusBar: WorkbenchStatusBar
  onOpenCaptures: () => Promise<void>
  openingCaptures: boolean
}) {
  const { t } = useTranslation()
  // 禁用数和熔断数都来自 hub 库,数据源挂了的时候监控服务把这两项直接置 0,
  // 只剩余额告急还是真的。这时候把三项加起来显示,就是拿一个结构性偏低的数
  // 当事实报出去(真有 21 个禁用 + 3 个熔断也会显示成「1 个信号」)。
  // 算不出来就不显示,不摆一个算错的数。
  const supplySignals = statusBar.hub_db_error
    ? null
    : statusBar.disabled_channels +
      statusBar.breaking_channels +
      statusBar.low_balance_sites
  const areas: ManagementArea[] = [
    {
      titleKey: 'Channels & supply',
      icon: Route,
      signalCount: supplySignals,
      links: [
        { labelKey: 'Channels', target: { to: '/channels' } },
        {
          labelKey: 'Channel Circuit Breaker',
          target: {
            to: '/system-settings/operations/$section',
            section: 'channel-breaker',
          },
        },
        {
          labelKey: 'Models',
          target: { to: '/models/$section', section: 'metadata' },
        },
      ],
    },
    {
      titleKey: 'Customers & requests',
      icon: Users,
      links: [
        { labelKey: 'Users', target: { to: '/users' } },
        {
          labelKey: 'Usage Logs',
          target: { to: '/usage-logs/$section', section: 'common' },
        },
        { labelKey: 'Conversation captures', action: 'captures' },
      ],
    },
    {
      titleKey: 'Pricing & revenue',
      icon: CircleDollarSign,
      links: [
        {
          labelKey: 'Model Pricing',
          target: {
            to: '/system-settings/billing/$section',
            section: 'model-pricing',
          },
        },
        {
          labelKey: 'Group Pricing',
          target: {
            to: '/system-settings/billing/$section',
            section: 'group-pricing',
          },
        },
        {
          labelKey: 'User pricing',
          target: {
            to: '/system-settings/billing/$section',
            section: 'user-pricing-override',
          },
        },
      ],
    },
    {
      titleKey: 'Platform & safety',
      icon: Activity,
      links: [
        { labelKey: 'System Info', target: { to: '/system-info' } },
        {
          labelKey: 'Performance',
          target: {
            to: '/system-settings/operations/$section',
            section: 'performance',
          },
        },
        {
          labelKey: 'Rate Limiting',
          target: {
            to: '/system-settings/security/$section',
            section: 'rate-limit',
          },
        },
      ],
    },
  ]

  return (
    <Card size='sm' className='h-full gap-2'>
      <CardHeader>
        <CardTitle>{t('Management areas')}</CardTitle>
      </CardHeader>
      <CardContent className='bg-border grid gap-px border-t px-0 sm:grid-cols-2'>
        {areas.map((area) => (
          <ManagementAreaRow
            key={area.titleKey}
            area={area}
            onOpenCaptures={onOpenCaptures}
            openingCaptures={openingCaptures}
          />
        ))}
      </CardContent>
    </Card>
  )
}
