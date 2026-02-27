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
  const [draggedIndex, setDraggedIndex] = useState(null);
  const [dragOverIndex, setDragOverIndex] = useState(null);

  const MAX_IMAGES = 20;
  const MAX_FILE_SIZE = 15 * 1024 * 1024; // 15MB
  const ACCEPTED_FORMATS = ['image/jpeg', 'image/png', 'image/webp'];

  // 验证文件
  const validateFile = (file) => {
    if (!ACCEPTED_FORMATS.includes(file.type)) {
      Toast.error(`不支持的文件格式: ${file.name}。仅支持 JPEG、PNG、WebP 格式`);
      return false;
    }
    if (file.size > MAX_FILE_SIZE) {
      Toast.error(`文件过大: ${file.name}。最大支持 15MB`);
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

  // 图片拖拽排序
  const handleImageDragStart = (e, index) => {
    setDraggedIndex(index);
    e.dataTransfer.effectAllowed = 'move';
    // 设置拖拽数据，防止与文件拖拽冲突
    e.dataTransfer.setData('text/plain', 'reorder');
  };

  const handleImageDragOver = (e, index) => {
    e.preventDefault();
    e.stopPropagation();
    
    // 检查是否是图片重排序（而非文件上传）
    if (draggedIndex === null) return;
    
    e.dataTransfer.dropEffect = 'move';
    setDragOverIndex(index);
  };

  const handleImageDragLeave = () => {
    setDragOverIndex(null);
  };

  const handleImageDrop = (e, dropIndex) => {
    e.preventDefault();
    e.stopPropagation();
    
    // 检查是否是图片重排序
    if (draggedIndex === null || draggedIndex === dropIndex) {
      setDraggedIndex(null);
      setDragOverIndex(null);
      return;
    }

    const newImages = [...referenceImages];
    const [draggedImage] = newImages.splice(draggedIndex, 1);
    newImages.splice(dropIndex, 0, draggedImage);
    
    onImagesChange(newImages);
    setDraggedIndex(null);
    setDragOverIndex(null);
  };

  const handleImageDragEnd = () => {
    setDraggedIndex(null);
    setDragOverIndex(null);
  };

  return (
    <div className='mb-6'>
      <div className='flex items-center justify-between mb-3'>
        <Text strong>参考图片（可选）</Text>
        <Text type='tertiary' size='small'>
          {referenceImages.length} / {MAX_IMAGES}
        </Text>
      </div>

      {/* 拖拽排序提示 */}
      <Text type='tertiary' size='small' className='block mb-3'>
        💡 提示：可以拖拽图片更改顺序
      </Text>

      {/* 图片预览列表 */}
      {referenceImages.length > 0 && (
        <div className='mb-3 grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3'>
          {referenceImages.map((image, index) => (
            <div
              key={image.id}
              draggable
              onDragStart={(e) => handleImageDragStart(e, index)}
              onDragOver={(e) => handleImageDragOver(e, index)}
              onDragLeave={handleImageDragLeave}
              onDrop={(e) => handleImageDrop(e, index)}
              onDragEnd={handleImageDragEnd}
              className={`
                relative group aspect-square rounded-lg overflow-hidden border bg-[var(--semi-color-fill-0)] transition-all cursor-move
                ${draggedIndex === index ? 'opacity-50 scale-95' : ''}
                ${dragOverIndex === index && draggedIndex !== null && draggedIndex !== index 
                  ? 'border-2 border-[var(--semi-color-primary)] scale-105' 
                  : 'border-[var(--semi-color-border)] hover:border-[var(--semi-color-primary)]'
                }
              `}
            >
              <img
                src={image.url}
                alt={image.name}
                className='w-full h-full object-cover pointer-events-none'
              />
              {/* 文件名提示 */}
              <div className='absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/70 to-transparent p-2 opacity-0 group-hover:opacity-100 transition-opacity'>
                <Text
                  size='small'
                  className='text-white truncate block'
                  title={image.name}
                >
                  {image.name}
                </Text>
              </div>
              {/* 删除按钮 - 始终显示在移动端，桌面端 hover 显示 */}
              <Button
                icon={<IconClose />}
                type='danger'
                theme='solid'
                size='small'
                className='absolute top-2 right-2 md:opacity-0 md:group-hover:opacity-100 transition-opacity shadow-lg'
                onClick={(e) => {
                  e.stopPropagation();
                  handleRemoveImage(image.id);
                }}
              />
            </div>
          ))}

          {/* 添加更多按钮 */}
          {referenceImages.length < MAX_IMAGES && (
            <div
              onClick={handleClick}
              className='aspect-square rounded-lg border-2 border-dashed border-[var(--semi-color-border)] hover:border-[var(--semi-color-primary)] bg-[var(--semi-color-fill-0)] hover:bg-[var(--semi-color-fill-1)] cursor-pointer transition-all flex flex-col items-center justify-center gap-2'
            >
              <IconImage size='extra-large' className='text-[var(--semi-color-text-2)]' />
              <Text type='tertiary' size='small'>
                添加图片
              </Text>
            </div>
          )}
        </div>
      )}

      {/* 初始上传区域 - 仅在没有图片时显示 */}
      {referenceImages.length === 0 && (
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
          <Text type='tertiary' size='small' className='block'>
            支持 JPEG、PNG、WebP 格式，单张最大 15MB，最多 20 张
          </Text>
        </div>
      )}

      <input
        ref={fileInputRef}
        type='file'
        accept='image/jpeg,image/png,image/webp'
        multiple
        onChange={handleFileInputChange}
        className='hidden'
      />
    </div>
  );
};

export default ReferenceImageSection;
