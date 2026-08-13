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
export const HOME_PAGE_THEMES = [
  { value: 'default', label: 'Default homepage' },
  // 下拉框里给站长看的是「长什么样」,不是主题的内部代号。
  // 'prism' 只作为 value 留在配置里。
  { value: 'prism', label: 'Dark developer homepage' },
] as const

export type HomePageTheme = (typeof HOME_PAGE_THEMES)[number]['value']

export function resolveHomePageTheme(value: unknown): HomePageTheme {
  return value === 'prism' ? 'prism' : 'default'
}
