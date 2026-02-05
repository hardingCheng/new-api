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
  return (
    <div className='mb-4'>
      <div className='flex items-center gap-2 mb-2'>
        <IconKey className='text-[var(--semi-color-text-2)]' />
        <Text strong>选择令牌</Text>
        {loading && <Spin size='small' />}
      </div>

      <Select
        value={selectedToken?.value}
        onChange={(value) => {
          const token = availableTokens.find((t) => t.value === value);
          onChange(token);
        }}
        placeholder='请选择令牌'
        optionList={availableTokens}
        loading={loading}
        filter
        className='w-full'
        emptyContent={loading ? '加载中...' : '暂无可用令牌，请先创建令牌'}
      />

      {availableTokens.length === 0 && !loading && (
        <Text type='warning' size='small' className='mt-1 block'>
          请先在令牌管理中创建令牌
        </Text>
      )}
    </div>
  );
};

export default TokenSelector;
