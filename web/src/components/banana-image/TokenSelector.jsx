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

import React from 'react';
import { Select, Typography, Spin } from '@douyinfe/semi-ui';
import { IconKey } from '@douyinfe/semi-icons';

const { Text } = Typography;

const TokenSelector = ({ selectedToken, availableTokens, loading, onChange }) => {
  // 过滤出 group 以 "生图_" 开头的令牌
  const filteredTokens = availableTokens.filter((token) => {
    // 必须有 group 属性，且不能是 "default"，且必须以 "生图_" 开头
    return token.group && 
           token.group !== 'default' && 
           token.group.startsWith('生图_');
  });

  // 如果当前选中的令牌不在过滤后的列表中，清空选中状态
  const currentValue = filteredTokens.some((t) => t.value === selectedToken?.value)
    ? selectedToken?.value
    : undefined;

  return (
    <div className='mb-4'>
      <div className='flex items-center gap-2 mb-2'>
        <IconKey className='text-[var(--semi-color-text-2)]' />
        <Text strong>选择令牌</Text>
        {loading && <Spin size='small' />}
      </div>

      <Select
        value={currentValue}
        onChange={(value) => {
          const token = filteredTokens.find((t) => t.value === value);
          onChange(token);
        }}
        placeholder='请选择令牌'
        optionList={filteredTokens.map((token) => ({
          ...token,
          label: token.name,
        }))}
        loading={loading}
        filter
        className='w-full'
        emptyContent={loading ? '加载中...' : '暂无可用的生图令牌，请先创建生图_开头的令牌组'}
      />

      {filteredTokens.length === 0 && !loading && (
        <Text type='warning' size='small' className='mt-1 block'>
          请先在令牌管理中创建属于"生图_"开头分组的令牌
        </Text>
      )}
    </div>
  );
};

export default TokenSelector;
