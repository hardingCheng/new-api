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

import { UPSTREAM_BALANCES_ANCHOR, type AlarmDestination } from '../management'

/** 报警去处 → 链接元素(Button 的 render 接元素,不接组件)。
 *  站内跳转必须走 <Link>:裸 <a> 指向站内路径会重载整个后台 SPA。
 *
 *  用穷尽 switch 而不是 if 链 —— 以后给 AlarmDestination 加一种去处,
 *  这里会编译报错,而不是静默落进锚点分支把人滚到余额表。 */
export function alarmDestinationLink(destination: AlarmDestination) {
  switch (destination.kind) {
    case 'channels':
      return <Link to='/channels' search={{ filter: destination.filter }} />
    case 'group-pricing':
      return (
        <Link
          to='/system-settings/billing/$section'
          params={{ section: 'group-pricing' }}
        />
      )
    case 'upstream-balances':
      // 同页锚点:原生锚点滚动就够,用 Link 会往 history 里多压一条。
      return <a href={`#${UPSTREAM_BALANCES_ANCHOR}`} />
    default: {
      const unhandled: never = destination
      throw new Error(
        `unhandled alarm destination: ${JSON.stringify(unhandled)}`
      )
    }
  }
}
