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
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { useStatus } from '@/hooks/use-status'

import { PrismCodeSample } from './prism-code-sample'
import {
  PrismEcosystem,
  PrismEcosystemMarquee,
  PrismModelShowcase,
  PrismOnboarding,
} from './prism-details'
import { PrismHero } from './prism-hero'
import { PrismTrust } from './prism-trust'
import {
  PrismCapabilities,
  PrismFinalCTA,
  PrismProviderBand,
  PrismRouteDisclosure,
} from './prism-sections'

import './prism-theme.css'

type PrismHomeProps = {
  isAuthenticated: boolean
}

export function PrismHome(props: PrismHomeProps) {
  const { status } = useStatus()
  const docsUrl =
    (status?.docs_link as string | undefined) || 'https://docs.newapi.pro'

  return (
    <div className='prism-theme dark'>
      <PublicLayout
        showMainContainer={false}
        showThemeSwitch={false}
        showNotifications={false}
      >
        <main className='bg-background text-foreground'>
          <PrismHero
            isAuthenticated={props.isAuthenticated}
            docsUrl={docsUrl}
          />
          <PrismProviderBand />
          <PrismTrust />
          <PrismModelShowcase />
          <PrismCodeSample />
          <PrismCapabilities />
          <PrismRouteDisclosure />
          <PrismOnboarding />
          <PrismEcosystem docsUrl={docsUrl} />
          <PrismEcosystemMarquee />
          <PrismFinalCTA isAuthenticated={props.isAuthenticated} />
        </main>
        <Footer />
      </PublicLayout>
    </div>
  )
}
