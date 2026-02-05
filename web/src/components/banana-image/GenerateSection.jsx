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
import { Button, Typography } from '@douyinfe/semi-ui';
import { IconImage } from '@douyinfe/semi-icons';

const { Text } = Typography;

const GenerateSection = ({
  onGenerate,
  isGenerating,
  disabled,
  currentSize,
  numberOfImages,
}) => {
  return (
    <div className='mb-6'>
      <div className='flex flex-col sm:flex-row items-stretch sm:items-center gap-4'>
        <Button
          theme='solid'
          type='primary'
          size='large'
          icon={<IconImage />}
          loading={isGenerating}
          disabled={disabled || isGenerating}
          onClick={onGenerate}
          className='flex-1 sm:flex-none sm:min-w-[200px] h-12'
        >
          {isGenerating ? '生成中...' : '🍌 生成图像'}
        </Button>

        <div className='text-center sm:text-left'>
          <Text type='tertiary'>
            将生成 {numberOfImages} 张 {currentSize.width}×{currentSize.height} 的图像
          </Text>
        </div>
      </div>
    </div>
  );
};

export default GenerateSection;
