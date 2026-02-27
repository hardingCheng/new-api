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

import React, { useState, useEffect } from 'react';
import {
  Typography,
  Spin,
  Button,
  Toast,
  Empty,
  Image,
} from '@douyinfe/semi-ui';
import {
  IconDownload,
  IconCopy,
  IconRefresh,
  IconDelete,
} from '@douyinfe/semi-icons';
import { GENERATION_STATUS } from '../../constants/banana-image.constants';
import { downloadImage } from '../../utils/imageCache';

const { Text, Title } = Typography;

const ResultSection = ({
  status,
  error,
  images,
  selectedIndex,
  onSelectImage,
  onReset,
  prompt,
  startTime,
  isMobile = false,
}) => {
  const [loadingDots, setLoadingDots] = useState('');

  // 动画点点点效果
  useEffect(() => {
    if (status === GENERATION_STATUS.LOADING) {
      const timer = setInterval(() => {
        setLoadingDots((prev) => (prev.length >= 3 ? '' : prev + '.'));
      }, 500);

      return () => clearInterval(timer);
    } else {
      setLoadingDots('');
    }
  }, [status]);

  // 下载图像
  const handleDownload = async (url, index) => {
    const filename = `banana-image-${Date.now()}-${index + 1}.png`;
    const success = await downloadImage(url, filename);
    if (success) {
      Toast.success('图像下载成功');
    } else {
      Toast.error('下载失败，请右键另存为');
    }
  };

  // 复制提示词
  const handleCopyPrompt = (text) => {
    if (text) {
      navigator.clipboard.writeText(text);
      Toast.success('提示词已复制');
    }
  };

  // 空状态
  if (status === GENERATION_STATUS.IDLE) {
    return (
      <div className='h-full flex items-center justify-center'>
        <div className='flex flex-col items-center gap-4'>
          <div className='flex items-center justify-center w-20 h-20 md:w-24 md:h-24 rounded-2xl bg-[#FFF9E6]'>
            <svg className='w-10 h-10 md:w-12 md:h-12 animate-bounce' viewBox='0 0 64 64' fill='none'>
              <path d='M45 8C45 8 48 8 50 10C52 12 52 15 52 15C52 15 52 18 50 20C48 22 45 22 45 22' stroke='#8B6914' strokeWidth='2' strokeLinecap='round'/>
              <path d='M45 10C45 10 42 12 40 18C38 24 36 32 34 38C32 44 28 52 22 56C16 60 10 58 8 54C6 50 8 44 12 40C16 36 22 34 28 32C34 30 40 28 44 24C48 20 50 14 50 10' fill='#FFD93D' stroke='#F4B400' strokeWidth='2' strokeLinecap='round' strokeLinejoin='round'/>
              <ellipse cx='28' cy='38' rx='3' ry='2' fill='#8B6914' opacity='0.2'/>
              <ellipse cx='20' cy='46' rx='2.5' ry='1.5' fill='#8B6914' opacity='0.2'/>
            </svg>
          </div>
          <div className='text-center'>
            <div className='text-[var(--semi-color-text-0)] text-base md:text-lg font-medium mb-2'>
              开始创作你的图片
            </div>
            <div className='text-[var(--semi-color-text-2)] text-sm'>
              在左侧配置面板输入提示词进行文生图，或上传参考图片进行图生图
            </div>
          </div>
        </div>
      </div>
    );
  }

  // 加载状态
  if (status === GENERATION_STATUS.LOADING) {
    return (
      <div className='h-full flex items-center justify-center'>
        <div className='flex flex-col items-center gap-4 md:gap-6'>
          <div className='relative'>
            <Spin size={isMobile ? 'default' : 'large'} />
            <div className='absolute -bottom-2 left-1/2 -translate-x-1/2'>
              <span className={`${isMobile ? 'text-2xl' : 'text-4xl'} animate-bounce`}>🍌</span>
            </div>
          </div>
          <div className='text-center px-4'>
            <Text className={`block ${isMobile ? 'text-base' : 'text-lg'} font-medium`}>
              正在生成图像{loadingDots}
            </Text>
            <Text type='tertiary' size='small' className='mt-2'>
              AI 正在创作中，请稍候
            </Text>
          </div>
        </div>
      </div>
    );
  }

  // 错误状态
  if (status === GENERATION_STATUS.ERROR) {
    return (
      <div className='h-full flex items-center justify-center'>
        <div className='flex flex-col items-center gap-3 md:gap-4 p-4 md:p-8 max-w-md text-center'>
          <div className={isMobile ? 'text-4xl' : 'text-6xl'}>❌</div>
          <Title heading={isMobile ? 6 : 5} type='danger'>
            生成失败
          </Title>
          <Text type='danger' className='break-all text-sm md:text-base'>
            {error || '未知错误'}
          </Text>
          <Button 
            onClick={onReset} 
            icon={<IconRefresh />} 
            theme='solid'
            size={isMobile ? 'small' : 'default'}
          >
            重试
          </Button>
        </div>
      </div>
    );
  }

  // 成功状态
  if (status === GENERATION_STATUS.SUCCESS && images.length > 0) {
    // 准备图片预览组
    const imageUrls = images.map(img => img.url);

    return (
      <div className='h-full flex flex-col'>
        {/* 缩略图网格 */}
        <div className={`grid ${images.length === 1 ? 'grid-cols-1' : isMobile ? 'grid-cols-2' : 'grid-cols-2 lg:grid-cols-3'} gap-3 md:gap-4`}>
          {images.map((img, index) => (
            <div key={img.id || index} className='relative group'>
              {/* 使用 Semi Design Image 组件，支持预览 */}
              <Image
                src={img.url}
                alt={`Generated ${index + 1}`}
                width='100%'
                height={isMobile ? 150 : 200}
                className='rounded-lg object-cover'
                preview={{
                  src: img.url,
                  visible: false,
                  getPopupContainer: () => document.body,
                  // 支持图片组预览
                  ...(images.length > 1 && {
                    previewSrcList: imageUrls,
                    currentIndex: index,
                  }),
                }}
              />

              {/* 操作按钮悬浮层 */}
              <div className='absolute inset-0 bg-black/0 group-hover:bg-black/40 transition-all rounded-lg flex items-center justify-center gap-1 md:gap-2 opacity-0 group-hover:opacity-100'>
                <Button
                  icon={<IconDownload />}
                  theme='solid'
                  size={isMobile ? 'small' : 'default'}
                  onClick={(e) => {
                    e.stopPropagation();
                    handleDownload(img.url, index);
                  }}
                />
                {img.revisedPrompt && (
                  <Button
                    icon={<IconCopy />}
                    theme='solid'
                    size={isMobile ? 'small' : 'default'}
                    onClick={(e) => {
                      e.stopPropagation();
                      handleCopyPrompt(img.revisedPrompt);
                    }}
                  />
                )}
              </div>

              {/* 图片序号标签 */}
              {images.length > 1 && (
                <div className='absolute top-2 left-2 bg-black/60 backdrop-blur-sm text-white text-xs px-2 py-1 rounded'>
                  {index + 1}/{images.length}
                </div>
              )}
            </div>
          ))}
        </div>

        {/* 原始提示词 */}
        {prompt && (
          <div className='mt-3 md:mt-4 p-2 md:p-3 bg-[var(--semi-color-fill-0)] rounded-lg'>
            <div className='flex items-start justify-between gap-2'>
              <Text type='secondary' size='small' className='text-xs md:text-sm flex-1'>
                <strong>原始提示词：</strong>
                {prompt}
              </Text>
              <Button
                icon={<IconCopy />}
                size='small'
                theme='borderless'
                onClick={() => handleCopyPrompt(prompt)}
              />
            </div>
          </div>
        )}

        {/* 修订后的提示词（显示第一张图的） */}
        {images[0]?.revisedPrompt && (
          <div className='mt-2 md:mt-3 p-2 md:p-3 bg-[var(--semi-color-fill-0)] rounded-lg'>
            <div className='flex items-start justify-between gap-2'>
              <Text type='secondary' size='small' className='text-xs md:text-sm flex-1'>
                <strong>优化后的提示词：</strong>
                {images[0].revisedPrompt}
              </Text>
              <Button
                icon={<IconCopy />}
                size='small'
                theme='borderless'
                onClick={() => handleCopyPrompt(images[0].revisedPrompt)}
              />
            </div>
          </div>
        )}

        {/* 重置按钮 */}
        <div className='mt-3 md:mt-4 flex justify-center'>
          <Button
            icon={<IconDelete />}
            onClick={onReset}
            size={isMobile ? 'small' : 'default'}
          >
            清除结果
          </Button>
        </div>
      </div>
    );
  }

  return null;
};

export default ResultSection;
