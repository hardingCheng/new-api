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

import React, { useState } from 'react';
import { TextArea, Typography, Collapsible, Button } from '@douyinfe/semi-ui';
import { IconChevronDown, IconChevronUp } from '@douyinfe/semi-icons';

const { Text } = Typography;

const PromptSection = ({
  prompt,
  negativePrompt,
  onPromptChange,
  onNegativePromptChange,
  onGenerate,
  isGenerating,
}) => {
  const [showNegative, setShowNegative] = useState(false);

  const handleKeyDown = (e) => {
    // Ctrl/Cmd + Enter 快速生成
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      if (!isGenerating && prompt.trim()) {
        onGenerate();
      }
    }
  };

  return (
    <div className='mb-6'>
      {/* 正向提示词 */}
      <div className='mb-3'>
        <div className='flex items-center justify-between mb-2'>
          <Text strong>提示词</Text>
          <Text type='tertiary' size='small'>
            {prompt.length} / 4000
          </Text>
        </div>
        <TextArea
          value={prompt}
          onChange={onPromptChange}
          onKeyDown={handleKeyDown}
          placeholder='描述你想要生成的图像内容，例如：一只可爱的橘猫躺在阳光下的窗台上，柔和的光线，高清摄影'
          autosize
          rows={4}
          maxCount={4000}
          showClear
          className='w-full'
        />
      </div>

      {/* 反向提示词（可折叠） */}
      <div>
        <Button
          theme='borderless'
          type='tertiary'
          size='small'
          icon={showNegative ? <IconChevronUp /> : <IconChevronDown />}
          iconPosition='right'
          onClick={() => setShowNegative(!showNegative)}
          className='px-0'
        >
          反向提示词（可选）
        </Button>

        <Collapsible isOpen={showNegative}>
          <div className='mt-2'>
            <TextArea
              value={negativePrompt}
              onChange={onNegativePromptChange}
              placeholder='描述你不想在图像中出现的元素，例如：模糊、低质量、变形、水印'
              autosize
              rows={2}
              maxCount={2000}
              showClear
              className='w-full'
            />
            <Text type='tertiary' size='small' className='mt-1 block'>
              注意：部分模型（如 DALL-E）不支持反向提示词
            </Text>
          </div>
        </Collapsible>
      </div>
    </div>
  );
};

export default PromptSection;
