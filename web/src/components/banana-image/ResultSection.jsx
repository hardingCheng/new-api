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
  Modal,
} from '@douyinfe/semi-ui';
import {
  IconDownload,
  IconCopy,
  IconRefresh,
  IconExpand,
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
  const [previewVisible, setPreviewVisible] = useState(false);
  const [previewSrc, setPreviewSrc] = useState('');
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

  const selectedImage = images[selectedIndex];

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
  const handleCopyPrompt = () => {
    if (prompt) {
      navigator.clipboard.writeText(prompt);
      Toast.success('提示词已复制');
    }
  };

  // 放大查看
  const handlePreview = (url) => {
    setPreviewSrc(url);
    setPreviewVisible(true);
  };

  // 空状态
  if (status === GENERATION_STATUS.IDLE) {
    return (
      <div className='h-full flex items-center justify-center'>
        <Empty
          image={<div className={`${isMobile ? 'text-5xl' : 'text-8xl'} opacity-50`}>🖼️</div>}
          title={
            <span className='text-[var(--semi-color-text-2)]'>等待生成</span>
          }
          description={
            <span className='text-[var(--semi-color-text-3)] text-sm'>
              {isMobile ? '配置参数后点击生成' : '在左侧配置参数后点击生成按钮开始创作'}
            </span>
          }
        />
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
    return (
      <div className='h-full flex flex-col'>
        {/* 主图预览 */}
        <div className='flex-1 relative bg-[var(--semi-color-fill-0)] rounded-lg md:rounded-xl overflow-hidden flex items-center justify-center min-h-[200px]'>
          <img
            src={selectedImage?.url}
            alt='Generated image'
            className='max-w-full max-h-full object-contain'
          />

          {/* 操作按钮悬浮层 */}
          <div className={`absolute ${isMobile ? 'bottom-2' : 'bottom-4'} left-1/2 -translate-x-1/2 flex gap-1 md:gap-2 bg-black/60 backdrop-blur-sm rounded-full px-2 md:px-4 py-1.5 md:py-2`}>
            <Button
              icon={<IconExpand />}
              theme='borderless'
              size={isMobile ? 'small' : 'default'}
              className='!text-white hover:!bg-white/20'
              onClick={() => handlePreview(selectedImage?.url)}
            />
            <Button
              icon={<IconDownload />}
              theme='borderless'
              size={isMobile ? 'small' : 'default'}
              className='!text-white hover:!bg-white/20'
              onClick={() => handleDownload(selectedImage?.url, selectedIndex)}
            />
            <Button
              icon={<IconCopy />}
              theme='borderless'
              size={isMobile ? 'small' : 'default'}
              className='!text-white hover:!bg-white/20'
              onClick={handleCopyPrompt}
            />
            <Button
              icon={<IconDelete />}
              theme='borderless'
              size={isMobile ? 'small' : 'default'}
              className='!text-white hover:!bg-white/20'
              onClick={onReset}
            />
          </div>
        </div>

        {/* 多图缩略图 */}
        {images.length > 1 && (
          <div className={`flex gap-2 md:gap-3 mt-3 md:mt-4 justify-center ${isMobile ? 'overflow-x-auto pb-2' : ''}`}>
            {images.map((img, index) => (
              <button
                key={img.id || index}
                type='button'
                onClick={() => onSelectImage(index)}
                className={`
                  flex-shrink-0 ${isMobile ? 'w-12 h-12' : 'w-16 h-16'} rounded-lg overflow-hidden border-2 transition-all
                  ${
                    index === selectedIndex
                      ? 'border-[var(--semi-color-primary)] ring-2 ring-[var(--semi-color-primary-light-default)]'
                      : 'border-transparent hover:border-[var(--semi-color-border)]'
                  }
                `}
              >
                <img
                  src={img.url}
                  alt={`Generated ${index + 1}`}
                  className='w-full h-full object-cover'
                />
              </button>
            ))}
          </div>
        )}

        {/* 修订后的提示词 */}
        {selectedImage?.revisedPrompt && (
          <div className='mt-3 md:mt-4 p-2 md:p-3 bg-[var(--semi-color-fill-0)] rounded-lg'>
            <Text type='secondary' size='small' className='text-xs md:text-sm'>
              <strong>优化后的提示词：</strong>
              {selectedImage.revisedPrompt}
            </Text>
          </div>
        )}

        {/* 图片预览 Modal */}
        <Modal
          visible={previewVisible}
          onCancel={() => setPreviewVisible(false)}
          footer={null}
          width={isMobile ? '95vw' : '90vw'}
          style={{ maxWidth: isMobile ? '100%' : '1200px' }}
          bodyStyle={{ padding: 0 }}
          closable
          fullScreen={isMobile}
        >
          <img src={previewSrc} alt='Preview' className='w-full h-auto' />
        </Modal>
      </div>
    );
  }

  return null;
};

export default ResultSection;
