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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/design-system/tabs'

import type { BillingSettings } from '../types'
import { ModelQuotaPoolSection } from './model-quota-pool-section'
import { UserPricingOverrideSection } from './user-pricing-override-section'
import { VideoBillingModeSection } from './video-billing-mode-section'

export type AdvancedPricingTab =
  | 'video-billing-mode'
  | 'model-quota-pool'
  | 'user-pricing-override'

export function AdvancedPricingSection(props: {
  settings: BillingSettings
  initialTab?: AdvancedPricingTab
}) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<AdvancedPricingTab>(
    props.initialTab ?? 'video-billing-mode'
  )

  return (
    <Tabs
      value={activeTab}
      onValueChange={(value) => setActiveTab(value as AdvancedPricingTab)}
      className='min-w-0'
    >
      <TabsList
        variant='line'
        className='h-auto w-full justify-start overflow-x-auto'
      >
        <TabsTrigger value='video-billing-mode' className='whitespace-nowrap'>
          {t('Video Billing Mode')}
        </TabsTrigger>
        <TabsTrigger value='model-quota-pool' className='whitespace-nowrap'>
          {t('Model Quota Pools')}
        </TabsTrigger>
        <TabsTrigger
          value='user-pricing-override'
          className='whitespace-nowrap'
        >
          {t('User Pricing Overrides')}
        </TabsTrigger>
      </TabsList>

      <TabsContent value='video-billing-mode' className='mt-4'>
        <VideoBillingModeSection
          defaultValue={props.settings.VideoBillingMode}
        />
      </TabsContent>
      <TabsContent value='model-quota-pool' className='mt-4'>
        <ModelQuotaPoolSection defaultValue={props.settings.ModelQuotaPool} />
      </TabsContent>
      <TabsContent value='user-pricing-override' className='mt-4'>
        <UserPricingOverrideSection
          defaultValue={props.settings.UserPricingOverride}
        />
      </TabsContent>
    </Tabs>
  )
}
