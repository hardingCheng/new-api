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
import { Button, Typography, Toast } from '@douyinfe/semi-ui';
import { IconImage } from '@douyinfe/semi-icons';

const { Text } = Typography;

const GenerateSection = ({
  onGenerate,
  isGenerating,
  disabled,
  currentSize,
  numberOfImages,
  prompt,
  selectedModel,
  selectedToken,
  resolution,
  aspectRatio,
  generationProgress,
}) => {
  const getDisabledReason = () => {
    const reasons = [];
    if (!selectedToken) {
      reasons.push('请选择令牌');
    }
    if (!selectedModel) {
      reasons.push('请选择模型');
    }
    if (!prompt?.trim()) {
      reasons.push('请输入提示词');
    }
    return reasons;
  };

  const handleClick = () => {
    if (disabled && !isGenerating) {
      const reasons = getDisabledReason();
      if (reasons.length > 0) {
        Toast.warning({
          content: (
            <div>
              <div className='font-semibold mb-1'>无法生成图像</div>
              <div>必填项：</div>
              <ul className='list-disc list-inside mt-1'>
                {reasons.map((reason, index) => (
                  <li key={index}>{reason}</li>
                ))}
              </ul>
            </div>
          ),
          duration: 3,
        });
      }
    } else if (!disabled && !isGenerating) {
      onGenerate();
    }
  };

  return (
    <div className='mb-6'>
      <div className='flex flex-col sm:flex-row items-stretch sm:items-center gap-4'>
        <Button
          theme='solid'
          type='primary'
          size='large'
          icon={<IconImage />}
          loading={isGenerating}
          disabled={isGenerating}
          onClick={handleClick}
          className='flex-1 sm:flex-none sm:min-w-[200px] h-12'
          style={disabled && !isGenerating ? { opacity: 0.6, cursor: 'not-allowed' } : {}}
        >
          {isGenerating ? '生成中...' : '🍌 生成图像'}
        </Button>

        <div className='text-center sm:text-left'>
          {isGenerating && generationProgress ? (
            <div>
              <Text type='primary' strong>
                进度: {generationProgress.completed}/{generationProgress.total}
              </Text>
              <Text type='tertiary' size='small' className='ml-2'>
                成功 {generationProgress.succeeded} 张
                {generationProgress.failed > 0 && (
                  <Text type='danger' size='small' className='ml-1'>
                    · 失败 {generationProgress.failed} 张
                  </Text>
                )}
              </Text>
            </div>
          ) : (
            <Text type='tertiary'>
              将生成 {numberOfImages} 张 {resolution.toUpperCase()} {aspectRatio} 的图像
            </Text>
          )}
        </div>
      </div>
    </div>
  );
};

export default GenerateSection;
