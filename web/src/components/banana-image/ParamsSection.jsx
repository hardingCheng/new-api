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
import { Typography, RadioGroup, Radio, InputNumber, Tooltip } from '@douyinfe/semi-ui';
import {
  RESOLUTION_OPTIONS,
  ASPECT_RATIO_OPTIONS,
} from '../../constants/banana-image.constants';

const { Text } = Typography;

const ParamsSection = ({
  resolution,
  aspectRatio,
  numberOfImages,
  currentSize,
  onResolutionChange,
  onAspectRatioChange,
  onNumberOfImagesChange,
}) => {
  return (
    <div className='mb-6 space-y-4'>
      {/* 分辨率选择 */}
      <div>
        <Text strong className='block mb-2'>
          分辨率
        </Text>
        <RadioGroup
          type='button'
          value={resolution}
          onChange={(e) => onResolutionChange(e.target.value)}
        >
          {RESOLUTION_OPTIONS.map((option) => (
            <Tooltip content={option.description} key={option.key}>
              <Radio value={option.key}>{option.label}</Radio>
            </Tooltip>
          ))}
        </RadioGroup>
      </div>

      {/* 比例选择 */}
      <div>
        <Text strong className='block mb-2'>
          图像比例
        </Text>
        <div className='grid grid-cols-4 gap-2'>
          {ASPECT_RATIO_OPTIONS.map((option) => {
            const isSelected = aspectRatio === option.key;
            // 计算预览图形的尺寸，最大边为24px
            const maxSize = 24;
            let previewWidth, previewHeight;
            if (option.width >= option.height) {
              previewWidth = maxSize;
              previewHeight = Math.round((maxSize * option.height) / option.width);
            } else {
              previewHeight = maxSize;
              previewWidth = Math.round((maxSize * option.width) / option.height);
            }

            return (
              <Tooltip
                key={option.key}
                content={`${option.name} - ${option.description}`}
                position='top'
              >
                <button
                  type='button'
                  onClick={() => onAspectRatioChange(option.key)}
                  className={`
                    flex flex-col items-center justify-center gap-1 p-2 rounded-lg border-2 transition-all
                    ${
                      isSelected
                        ? 'border-[var(--semi-color-primary)] bg-[var(--semi-color-primary-light-default)]'
                        : 'border-[var(--semi-color-border)] hover:border-[var(--semi-color-primary-hover)] bg-[var(--semi-color-fill-0)]'
                    }
                  `}
                >
                  {/* 比例预览图形 */}
                  <div
                    className={`
                      rounded-sm transition-colors
                      ${isSelected ? 'bg-[var(--semi-color-primary)]' : 'bg-[var(--semi-color-text-3)]'}
                    `}
                    style={{
                      width: `${previewWidth}px`,
                      height: `${previewHeight}px`,
                    }}
                  />
                  {/* 标签 */}
                  <span
                    className={`
                      text-xs transition-colors
                      ${isSelected ? 'text-[var(--semi-color-primary)]' : 'text-[var(--semi-color-text-2)]'}
                    `}
                  >
                    {option.label}
                  </span>
                </button>
              </Tooltip>
            );
          })}
        </div>

        {/* 当前尺寸显示 */}
        <div className='mt-3 p-2 bg-[var(--semi-color-fill-0)] rounded-lg text-center'>
          <Text type='secondary' size='small'>
            输出尺寸: <Text strong>{currentSize.width} × {currentSize.height}</Text> px
          </Text>
        </div>
      </div>

      {/* 生成数量 */}
      <div>
        <Text strong className='block mb-2'>
          生成数量
        </Text>
        <div className='flex items-center gap-3'>
          <InputNumber
            value={numberOfImages}
            onChange={onNumberOfImagesChange}
            min={1}
            max={4}
            className='w-24'
          />
          <Text type='tertiary' size='small'>张（1-4张）</Text>
        </div>
      </div>
    </div>
  );
};

export default ParamsSection;
