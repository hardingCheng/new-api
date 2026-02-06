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

import React, { useRef, useState } from 'react';
import { Typography, Button, Toast } from '@douyinfe/semi-ui';
import { IconClose, IconImage } from '@douyinfe/semi-icons';

const { Text } = Typography;

const ReferenceImageSection = ({ referenceImages = [], onImagesChange }) => {
  const fileInputRef = useRef(null);
  const [isDragging, setIsDragging] = useState(false);

  const MAX_IMAGES = 3;
  const MAX_FILE_SIZE = 5 * 1024 * 1024; // 5MB
  const ACCEPTED_FORMATS = ['image/jpeg', 'image/png', 'image/webp'];

  // 验证文件
  const validateFile = (file) => {
    if (!ACCEPTED_FORMATS.includes(file.type)) {
      Toast.error(`不支持的文件格式: ${file.name}。仅支持 JPEG、PNG、WebP 格式`);
      return false;
    }
    if (file.size > MAX_FILE_SIZE) {
      Toast.error(`文件过大: ${file.name}。最大支持 5MB`);
      return false;
    }
    return true;
  };

  // 处理文件选择
  const handleFileSelect = async (files) => {
    const fileArray = Array.from(files);
    const remainingSlots = MAX_IMAGES - referenceImages.length;

    if (fileArray.length > remainingSlots) {
      Toast.warning(`最多只能上传 ${MAX_IMAGES} 张图片，当前还可以上传 ${remainingSlots} 张`);
      return;
    }

    const validFiles = fileArray.filter(validateFile);
    if (validFiles.length === 0) return;

    // 转换为 base64
    const newImages = await Promise.all(
      validFiles.map((file) => {
        return new Promise((resolve) => {
          const reader = new FileReader();
          reader.onload = (e) => {
            resolve({
              id: `${Date.now()}-${Math.random()}`,
              url: e.target.result,
              name: file.name,
            });
          };
          reader.readAsDataURL(file);
        });
      })
    );

    onImagesChange([...referenceImages, ...newImages]);
  };

  // 点击上传
  const handleClick = () => {
    if (referenceImages.length >= MAX_IMAGES) {
      Toast.warning(`最多只能上传 ${MAX_IMAGES} 张图片`);
      return;
    }
    fileInputRef.current?.click();
  };

  // 文件输入变化
  const handleFileInputChange = (e) => {
    const files = e.target.files;
    if (files && files.length > 0) {
      handleFileSelect(files);
    }
    // 清空 input，允许重复选择同一文件
    e.target.value = '';
  };

  // 拖拽事件
  const handleDragEnter = (e) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  };

  const handleDragLeave = (e) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  };

  const handleDragOver = (e) => {
    e.preventDefault();
    e.stopPropagation();
  };

  const handleDrop = (e) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);

    const files = e.dataTransfer.files;
    if (files && files.length > 0) {
      handleFileSelect(files);
    }
  };

  // 删除图片
  const handleRemoveImage = (id) => {
    onImagesChange(referenceImages.filter((img) => img.id !== id));
  };

  return (
    <div className='mb-6'>
      <div className='flex items-center justify-between mb-2'>
        <Text strong>参考图片（可选，1-3张）</Text>
        <Text type='tertiary' size='small'>
          {referenceImages.length} / {MAX_IMAGES}
        </Text>
      </div>

      {/* 上传区域 */}
      {referenceImages.length < MAX_IMAGES && (
        <div
          onClick={handleClick}
          onDragEnter={handleDragEnter}
          onDragLeave={handleDragLeave}
          onDragOver={handleDragOver}
          onDrop={handleDrop}
          className={`
            relative border-2 border-dashed rounded-lg p-4 md:p-6 text-center cursor-pointer transition-all
            ${
              isDragging
                ? 'border-[var(--semi-color-primary)] bg-[var(--semi-color-primary-light-default)]'
                : 'border-[var(--semi-color-border)] hover:border-[var(--semi-color-primary)] bg-[var(--semi-color-fill-0)]'
            }
          `}
        >
          <IconImage size='large' className='text-[var(--semi-color-text-2)] mb-2' />
          <Text type='secondary' className='block mb-1 text-sm md:text-base'>
            点击或拖拽图片到此处上传
          </Text>
          <Text type='tertiary' size='small' className='block text-xs md:text-sm'>
            支持 JPEG、PNG、WebP 格式，单张最大 5MB，1-3张
          </Text>

          <input
            ref={fileInputRef}
            type='file'
            accept='image/jpeg,image/png,image/webp'
            multiple
            onChange={handleFileInputChange}
            className='hidden'
          />
        </div>
      )}

      {/* 图片预览列表 */}
      {referenceImages.length > 0 && (
        <div className='mt-3 grid grid-cols-3 gap-2'>
          {referenceImages.map((image) => (
            <div
              key={image.id}
              className='relative group aspect-square rounded-lg overflow-hidden border border-[var(--semi-color-border)]'
            >
              <img
                src={image.url}
                alt={image.name}
                className='w-full h-full object-cover'
              />
              <div className='absolute inset-0 bg-black bg-opacity-0 group-hover:bg-opacity-40 transition-all flex items-center justify-center'>
                <Button
                  icon={<IconClose />}
                  type='danger'
                  theme='solid'
                  size='small'
                  className='opacity-0 group-hover:opacity-100 transition-opacity'
                  onClick={(e) => {
                    e.stopPropagation();
                    handleRemoveImage(image.id);
                  }}
                />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default ReferenceImageSection;
