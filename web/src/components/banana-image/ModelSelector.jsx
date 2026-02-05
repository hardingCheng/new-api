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
import { IconImage } from '@douyinfe/semi-icons';

const { Text } = Typography;

const ModelSelector = ({
  selectedModel,
  availableModels,
  loading,
  onChange,
  disabled,
}) => {
  return (
    <div className='mb-4'>
      <div className='flex items-center gap-2 mb-2'>
        <IconImage className='text-[var(--semi-color-text-2)]' />
        <Text strong>选择模型</Text>
        {loading && <Spin size='small' />}
      </div>

      <Select
        value={selectedModel}
        onChange={onChange}
        placeholder={disabled ? '请先选择令牌' : '请选择图像生成模型'}
        optionList={availableModels}
        loading={loading}
        filter
        className='w-full'
        disabled={disabled}
        showClear
        emptyContent={
          loading
            ? '加载中...'
            : disabled
            ? '请先选择令牌'
            : '该令牌下暂无可用的图像生成模型'
        }
      />

      {!disabled && availableModels.length === 0 && !loading && (
        <Text type='tertiary' size='small' className='mt-1 block'>
          未找到图像生成模型（如 dall-e、flux、imagen 等）
        </Text>
      )}
    </div>
  );
};

export default ModelSelector;
