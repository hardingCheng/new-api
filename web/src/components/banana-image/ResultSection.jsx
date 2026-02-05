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
} from '@douyinfe/semi-icons';
import { GENERATION_STATUS } from '../../constants/banana-image.constants';

const { Text, Title } = Typography;

const ResultSection = ({
  status,
  error,
  images,
  selectedIndex,
  onSelectImage,
  onReset,
  prompt,
}) => {
  const [previewVisible, setPreviewVisible] = useState(false);
  const [previewSrc, setPreviewSrc] = useState('');

  const selectedImage = images[selectedIndex];

  // 下载图像
  const handleDownload = async (url, index) => {
    try {
      const response = await fetch(url);
      const blob = await response.blob();
      const downloadUrl = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = downloadUrl;
      link.download = `banana-image-${Date.now()}-${index + 1}.png`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(downloadUrl);
      Toast.success('图像下载成功');
    } catch (err) {
      console.error('Download failed:', err);
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
          image={<div className='text-8xl opacity-50'>🖼️</div>}
          title={
            <span className='text-[var(--semi-color-text-2)]'>等待生成</span>
          }
          description={
            <span className='text-[var(--semi-color-text-3)]'>
              在左侧配置参数后点击生成按钮开始创作
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
        <div className='flex flex-col items-center gap-6'>
          <div className='relative'>
            <Spin size='large' />
            <div className='absolute -bottom-2 left-1/2 -translate-x-1/2'>
              <span className='text-4xl animate-bounce'>🍌</span>
            </div>
          </div>
          <div className='text-center'>
            <Text className='block text-lg'>正在生成图像...</Text>
            <Text type='tertiary' size='small' className='mt-2'>
              这可能需要几秒到几十秒不等
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
        <div className='flex flex-col items-center gap-4 p-8 max-w-md text-center'>
          <div className='text-6xl'>❌</div>
          <Title heading={5} type='danger'>
            生成失败
          </Title>
          <Text type='danger' className='break-all'>
            {error || '未知错误'}
          </Text>
          <Button onClick={onReset} icon={<IconRefresh />} theme='solid'>
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
        <div className='flex-1 relative bg-[var(--semi-color-fill-0)] rounded-xl overflow-hidden flex items-center justify-center'>
          <img
            src={selectedImage?.url}
            alt='Generated image'
            className='max-w-full max-h-full object-contain'
          />

          {/* 操作按钮悬浮层 */}
          <div className='absolute bottom-4 left-1/2 -translate-x-1/2 flex gap-2 bg-black/60 backdrop-blur-sm rounded-full px-4 py-2'>
            <Button
              icon={<IconExpand />}
              theme='borderless'
              className='!text-white hover:!bg-white/20'
              onClick={() => handlePreview(selectedImage?.url)}
            />
            <Button
              icon={<IconDownload />}
              theme='borderless'
              className='!text-white hover:!bg-white/20'
              onClick={() => handleDownload(selectedImage?.url, selectedIndex)}
            />
            <Button
              icon={<IconCopy />}
              theme='borderless'
              className='!text-white hover:!bg-white/20'
              onClick={handleCopyPrompt}
            />
          </div>
        </div>

        {/* 多图缩略图 */}
        {images.length > 1 && (
          <div className='flex gap-3 mt-4 justify-center'>
            {images.map((img, index) => (
              <button
                key={img.id || index}
                type='button'
                onClick={() => onSelectImage(index)}
                className={`
                  flex-shrink-0 w-16 h-16 rounded-lg overflow-hidden border-2 transition-all
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
          <div className='mt-4 p-3 bg-[var(--semi-color-fill-0)] rounded-lg'>
            <Text type='secondary' size='small'>
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
          width='90vw'
          style={{ maxWidth: '1200px' }}
          bodyStyle={{ padding: 0 }}
          closable
        >
          <img src={previewSrc} alt='Preview' className='w-full h-auto' />
        </Modal>
      </div>
    );
  }

  return null;
};

export default ResultSection;
