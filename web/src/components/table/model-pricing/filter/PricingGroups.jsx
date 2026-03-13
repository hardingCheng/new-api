/*
Copyright (C) 2025 QuantumNous

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

import React, { useMemo } from 'react';
import SelectableButtonGroup from '../../../common/ui/SelectableButtonGroup';

/**
 * 分组筛选组件
 * @param {string} filterGroup 当前选中的分组，'all' 表示不过滤
 * @param {Function} setFilterGroup 设置选中分组
 * @param {Record<string, any>} usableGroup 后端返回的可用分组对象
 * @param {Record<string, number>} groupRatio 分组倍率对象
 * @param {Array} models 模型列表
 * @param {boolean} loading 是否加载中
 * @param {Function} t i18n
 */
const PricingGroups = ({
  filterGroup,
  setFilterGroup,
  usableGroup = {},
  groupRatio = {},
  models = [],
  loading = false,
  t,
}) => {
  // 只显示用户可见的分组
  const userVisibleGroups = useMemo(() => {
    return Object.keys(usableGroup).filter((key) => key !== '');
  }, [usableGroup]);

  // 计算每个分组中所有模型的最高价格
  const calculateGroupMaxPrice = useMemo(() => {
    const groupMaxPrices = {};

    userVisibleGroups.forEach((group) => {
      let maxPrice = 0;
      const ratio = groupRatio[group] || 1;

      // 遍历所有模型，找出该分组中的最高价格
      models.forEach((model) => {
        // 只计算该模型在当前分组中可用的情况
        if (
          !model.enable_groups ||
          !model.enable_groups.includes(group)
        ) {
          return;
        }

        let modelPrice = 0;
        if (model.quota_type === 0) {
          // 按量计费：使用 completion_ratio 计算输出价格（通常更高）
          const completionPrice =
            model.model_ratio * (model.completion_ratio || 1) * 2 * ratio;
          modelPrice = completionPrice;
        } else if (model.quota_type === 1) {
          // 按次计费
          modelPrice = parseFloat(model.model_price || 0) * ratio;
        }

        if (modelPrice > maxPrice) {
          maxPrice = modelPrice;
        }
      });

      groupMaxPrices[group] = maxPrice;
    });

    return groupMaxPrices;
  }, [models, userVisibleGroups, groupRatio]);

  const items = useMemo(() => {
    const groups = ['all', ...userVisibleGroups];

    return groups.map((g) => {
      let ratioDisplay = '';
      if (g === 'all') {
        // 全部分组显示所有模型数量
        ratioDisplay = '';
      } else {
        // 显示该分组的最高价格
        const maxPrice = calculateGroupMaxPrice[g] || 0;
        if (maxPrice > 0) {
          ratioDisplay = `$${maxPrice.toFixed(3)}`;
        } else {
          const ratio = groupRatio[g];
          ratioDisplay = ratio !== undefined && ratio !== null ? `${ratio}x` : '1x';
        }
      }
      return {
        value: g,
        label: g === 'all' ? t('全部分组') : g,
        tagCount: ratioDisplay,
      };
    });
  }, [userVisibleGroups, calculateGroupMaxPrice, groupRatio, t]);

  return (
    <SelectableButtonGroup
      title={t('可用令牌分组')}
      items={items}
      activeValue={filterGroup}
      onChange={setFilterGroup}
      loading={loading}
      variant='teal'
      t={t}
    />
  );
};

export default PricingGroups;
