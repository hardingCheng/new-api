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
import { useTranslation } from 'react-i18next'

import { usePricingData } from './use-pricing-data'

// 后端把「用户自身注册分组」在 usable_group 里的说明固定为“用户分组”
// (service/group.go);该分组名是内部标识,用户可见处一律以中性名替代。
export function useGroupDisplayLabel() {
  const { t } = useTranslation()
  const { usableGroup } = usePricingData()
  return (group: string): string | undefined =>
    usableGroup?.[group]?.desc === '用户分组' ? t('User Group') : undefined
}
