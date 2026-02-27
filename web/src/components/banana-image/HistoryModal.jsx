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

import React, { useState, useMemo, useEffect } from 'react';
import {
  Modal,
  Typography,
  Button,
  Empty,
  Popconfirm,
  Tabs,
  TabPane,
  Toast,
  Input,
  Banner,
} from '@douyinfe/semi-ui';
import {
  IconDeleteStroked,
  IconDownload,
  IconDelete,
  IconSetting,
  IconSearch,
  IconInfoCircle,
  IconCopy,
} from '@douyinfe/semi-icons';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import 'dayjs/locale/zh-cn';
import { downloadImage, getCacheConfig } from '../../utils/imageCache';
import CacheSettings from './CacheSettings';

dayjs.extend(relativeTime);
dayjs.locale('zh-cn');

const { Text } = Typography;

const HistoryModal = ({
  visible,
  records,
  onSelect,
  onDelete,
  onClear,
  onClose,
  cacheStats,
  hasMore,
  onLoadMore,
  onSearch,
  totalCount,
  isLoading,
}) => {
  const [activeTab, setActiveTab] = useState('history');
  const [searchText, setSearchText] = useState('');
  const [cacheConfig, setCacheConfig] = useState(null);
  const [selectedIds, setSelectedIds] = useState([]);
  const [isSelectionMode, setIsSelectionMode] = useState(false);
  const [isSearching, setIsSearching] = useState(false);

  // 加载缓存配置
  useEffect(() => {
    if (visible) {
      const config = getCacheConfig();
      setCacheConfig(config);
    }
  }, [visible]);

  // 重置选择状态和搜索
  useEffect(() => {
    if (!visible) {
      setSelectedIds([]);
      setIsSelectionMode(false);
      setSearchText('');
      setIsSearching(false);
    }
  }, [visible]);

  // 处理搜索
  const handleSearch = async (value) => {
    setSearchText(value);
    if (value.trim()) {
      setIsSearching(true);
      await onSearch(value);
    } else {
      setIsSearching(false);
      // 清空搜索，重新加载第一页
      await onSearch('');
    }
  };

  // 切换选择模式
  const toggleSelectionMode = () => {
    setIsSelectionMode(!isSelectionMode);
    if (isSelectionMode) {
      setSelectedIds([]);
    }
  };

  // 切换单个记录的选择状态
  const toggleSelection = (id) => {
    setSelectedIds((prev) =>
      prev.includes(id) ? prev.filter((i) => i !== id) : [...prev, id]
    );
  };

  // 全选
  const selectAll = () => {
    setSelectedIds(records.map((r) => r.id));
  };

  // 反选
  const invertSelection = () => {
    const allIds = records.map((r) => r.id);
    setSelectedIds(allIds.filter((id) => !selectedIds.includes(id)));
  };

  // 批量删除
  const handleBatchDelete = async () => {
    // 逐个删除（这样会同时删除 IndexedDB 中的图片）
    for (const id of selectedIds) {
      await onDelete(id);
    }
    setSelectedIds([]);
    setIsSelectionMode(false);
    Toast.success(`已删除 ${selectedIds.length} 条记录`);
  };

  // 批量导出
  const handleBatchExport = async () => {
    const selectedRecords = records.filter((r) => selectedIds.includes(r.id));
    let exportedCount = 0;

    for (const record of selectedRecords) {
      if (record.images && record.images.length > 0) {
        for (let i = 0; i < record.images.length; i++) {
          const img = record.images[i];
          const filename = `zlai-image-${record.id}-${i + 1}.png`;
          const success = await downloadImage(img.url, filename);
          if (success) {
            exportedCount++;
          }
        }
      }
    }

    if (exportedCount > 0) {
      Toast.success(`已导出 ${exportedCount} 张图片`);
    } else {
      Toast.error('导出失败');
    }
  };

  const formatTime = (timestamp) => {
    return dayjs(timestamp).fromNow();
  };

  const formatSize = (bytes) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
  };

  const formatDays = (ms) => {
    return Math.floor(ms / (24 * 60 * 60 * 1000));
  };

  const handleDownloadImage = async (url) => {
    const filename = `banana-image-${Date.now()}.png`;
    const success = await downloadImage(url, filename);
    if (success) {
      Toast.success('图片下载成功');
    } else {
      Toast.error('图片下载失败');
    }
  };

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      footer={null}
      width='90vw'
      style={{ maxWidth: '800px' }}
      bodyStyle={{ padding: 0, height: '70vh' }}
      fullScreen={window.innerWidth < 768}
      title='历史记录'
    >
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        type='line'
        style={{ height: '100%' }}
      >
        <TabPane
          tab={
            <span>
              📜 历史记录 {totalCount > 0 && `(${totalCount})`}
            </span>
          }
          itemKey='history'
        >
          <div className='h-[calc(70vh-60px)] overflow-y-auto'>
            {/* 缓存设置提示 */}
            {cacheConfig && (
              <div className='p-4 pb-0'>
                <Banner
                  type='info'
                  icon={<IconInfoCircle />}
                  description={
                    <div className='flex items-center justify-between'>
                      <span>
                        当前缓存设置：保存 {formatDays(cacheConfig.maxAge)} 天 · 最多 {cacheConfig.maxCount} 张 · 最大 {formatSize(cacheConfig.maxSize)}
                      </span>
                      <Button
                        size='small'
                        theme='borderless'
                        onClick={() => setActiveTab('settings')}
                      >
                        去设置
                      </Button>
                    </div>
                  }
                  closeIcon={null}
                />
              </div>
            )}

            {totalCount === 0 ? (
              <div className='p-8'>
                <Empty
                  image={<div className='text-4xl'>📜</div>}
                  title='暂无历史记录'
                  description='生成的图像会保存在这里'
                />
              </div>
            ) : (
              <>
                {/* 搜索框和操作区域 */}
                <div className='p-4 pb-2 sticky top-0 bg-[var(--semi-color-bg-0)] z-10 space-y-3'>
                  <Input
                    prefix={<IconSearch />}
                    placeholder='搜索提示词（搜索全部记录）...'
                    value={searchText}
                    onChange={handleSearch}
                    showClear
                  />
                  
                  {/* 操作按钮区域 */}
                  <div className='flex items-center gap-2 p-3 bg-[var(--semi-color-fill-0)] rounded-lg border border-[var(--semi-color-border)]'>
                    <Text type='tertiary' size='small' className='mr-2'>
                      操作：
                    </Text>
                    
                    <Button
                      size='small'
                      theme='borderless'
                      onClick={toggleSelectionMode}
                      type={isSelectionMode ? 'primary' : 'tertiary'}
                    >
                      {isSelectionMode ? '✓ 批量选择' : '批量选择'}
                    </Button>
                    
                    {isSelectionMode && (
                      <>
                        <Button
                          size='small'
                          theme='borderless'
                          onClick={selectAll}
                          disabled={selectedIds.length === records.length}
                        >
                          全选
                        </Button>
                        <Button
                          size='small'
                          theme='borderless'
                          onClick={invertSelection}
                        >
                          反选
                        </Button>
                      </>
                    )}
                    
                    <div className='flex-1' />
                    
                    <Button
                      size='small'
                      theme='borderless'
                      icon={<IconDownload />}
                      onClick={handleBatchExport}
                      disabled={!isSelectionMode || selectedIds.length === 0}
                    >
                      导出 {isSelectionMode && selectedIds.length > 0 && `(${selectedIds.length})`}
                    </Button>
                    
                    <Popconfirm
                      title={isSelectionMode ? '确定要删除选中的记录吗？' : '确定要清空所有历史记录吗？'}
                      content='此操作不可恢复'
                      onConfirm={async () => {
                        if (isSelectionMode) {
                          await handleBatchDelete();
                        } else {
                          await onClear();
                          Toast.success('已清空历史记录');
                        }
                      }}
                    >
                      <Button
                        size='small'
                        theme='borderless'
                        type='danger'
                        icon={isSelectionMode ? <IconDelete /> : <IconDeleteStroked />}
                        disabled={isSelectionMode && selectedIds.length === 0}
                      >
                        {isSelectionMode 
                          ? `删除 ${selectedIds.length > 0 ? `(${selectedIds.length})` : ''}` 
                          : '清空全部'}
                      </Button>
                    </Popconfirm>
                  </div>
                </div>

                {/* 记录列表 */}
                {records.length === 0 && !isLoading ? (
                  <div className='p-4 md:p-8'>
                    <Empty
                      image={<div className='text-3xl md:text-4xl'>🔍</div>}
                      title='未找到匹配的记录'
                      description='尝试使用其他关键词搜索'
                    />
                  </div>
                ) : (
                  <>
                    <div className='p-3 md:p-4 pt-2 grid grid-cols-1 sm:grid-cols-2 gap-3 md:gap-4'>
                      {records.map((record) => (
                        <HistoryCard
                          key={record.id}
                          record={record}
                          isSelectionMode={isSelectionMode}
                          isSelected={selectedIds.includes(record.id)}
                          onToggleSelection={() => toggleSelection(record.id)}
                          onSelect={() => {
                            if (!isSelectionMode) {
                              onSelect(record);
                              onClose();
                            }
                          }}
                          onDelete={() => onDelete(record.id)}
                          onDownload={handleDownloadImage}
                          formatTime={formatTime}
                        />
                      ))}
                    </div>
                    
                    {/* 加载更多按钮 */}
                    {!isSearching && hasMore && (
                      <div className='p-4 flex justify-center'>
                        <Button
                          onClick={onLoadMore}
                          loading={isLoading}
                          disabled={isLoading}
                        >
                          {isLoading ? '加载中...' : '加载更多'}
                        </Button>
                      </div>
                    )}
                    
                    {/* 加载中提示 */}
                    {isLoading && records.length === 0 && (
                      <div className='p-8 text-center'>
                        <Text type='tertiary'>加载中...</Text>
                      </div>
                    )}
                    
                    {/* 没有更多数据提示 */}
                    {!hasMore && records.length > 0 && !isSearching && (
                      <div className='p-4 text-center'>
                        <Text type='tertiary' size='small'>已加载全部 {totalCount} 条记录</Text>
                      </div>
                    )}
                  </>
                )}
              </>
            )}
          </div>
        </TabPane>
        <TabPane
          tab={
            <span>
              <IconSetting /> 缓存设置
            </span>
          }
          itemKey='settings'
        >
          <div className='h-[calc(70vh-60px)] overflow-y-auto'>
            <CacheSettings cacheStats={cacheStats} />
          </div>
        </TabPane>
      </Tabs>
    </Modal>
  );
};

const HistoryCard = ({ 
  record, 
  isSelectionMode, 
  isSelected, 
  onToggleSelection, 
  onSelect, 
  onDelete, 
  onDownload, 
  formatTime 
}) => {
  const thumbnailUrl = record.images?.[0]?.url;
  const imageCount = record.images?.length || 0;

  const handleCardClick = () => {
    if (isSelectionMode) {
      onToggleSelection();
    } else {
      onSelect();
    }
  };

  return (
    <div 
      className={`group relative rounded-lg border transition-all overflow-hidden bg-[var(--semi-color-bg-1)] ${
        isSelected 
          ? 'border-[var(--semi-color-primary)] ring-2 ring-[var(--semi-color-primary)]' 
          : 'border-[var(--semi-color-border)] hover:border-[var(--semi-color-primary)]'
      }`}
    >
      {/* 缩略图 */}
      <div
        className='relative w-full h-32 sm:h-40 bg-[var(--semi-color-fill-1)] cursor-pointer'
        onClick={handleCardClick}
      >
        {thumbnailUrl ? (
          <img
            src={thumbnailUrl}
            alt='Thumbnail'
            className='w-full h-full object-cover'
          />
        ) : (
          <div className='w-full h-full flex items-center justify-center text-4xl'>
            🖼️
          </div>
        )}
        
        {/* 选择模式复选框 */}
        {isSelectionMode && (
          <div className='absolute top-2 left-2'>
            <div 
              className={`w-6 h-6 rounded border-2 flex items-center justify-center ${
                isSelected 
                  ? 'bg-[var(--semi-color-primary)] border-[var(--semi-color-primary)]' 
                  : 'bg-white/90 border-gray-400'
              }`}
            >
              {isSelected && (
                <svg className='w-4 h-4 text-white' fill='none' viewBox='0 0 24 24' stroke='currentColor'>
                  <path strokeLinecap='round' strokeLinejoin='round' strokeWidth={3} d='M5 13l4 4L19 7' />
                </svg>
              )}
            </div>
          </div>
        )}
        
        {/* 图片数量标签 */}
        {imageCount > 1 && (
          <div className='absolute top-2 right-2 bg-black/70 text-white px-2 py-1 rounded text-xs'>
            {imageCount} 张
          </div>
        )}

        {/* 悬浮操作按钮 */}
        {!isSelectionMode && (
          <div className='absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center gap-2'>
            <Button
              icon={<IconDownload />}
              theme='solid'
              size='small'
              onClick={(e) => {
                e.stopPropagation();
                onDownload(thumbnailUrl);
              }}
            >
              下载
            </Button>
          </div>
        )}
      </div>

      {/* 信息区域 */}
      <div className='p-3'>
        <div className='flex items-start gap-2 mb-2'>
          <Text
            ellipsis={{ showTooltip: true, rows: 2 }}
            className='flex-1 text-sm font-medium'
          >
            {record.prompt || '无提示词'}
          </Text>
          {record.prompt && (
            <Button
              icon={<IconCopy />}
              theme='borderless'
              size='small'
              onClick={(e) => {
                e.stopPropagation();
                navigator.clipboard.writeText(record.prompt);
                Toast.success('提示词已复制');
              }}
              style={{ flexShrink: 0 }}
            />
          )}
        </div>
        
        <div className='flex items-center justify-between text-xs'>
          <div className='flex items-center gap-2 flex-1 min-w-0'>
            <Text type='tertiary' size='small' ellipsis>
              {record.model?.split('/').pop() || '未知模型'}
            </Text>
            {record.params && (
              <>
                <Text type='tertiary' size='small'>•</Text>
                <Text type='tertiary' size='small'>
                  {record.params.width}×{record.params.height}
                </Text>
              </>
            )}
          </div>
          
          {!isSelectionMode && (
            <Popconfirm
              title='确定删除？'
              content='此操作不可恢复'
              onConfirm={(e) => {
                e?.stopPropagation();
                onDelete();
              }}
            >
              <Button
                icon={<IconDelete />}
                theme='borderless'
                type='danger'
                size='small'
                onClick={(e) => e.stopPropagation()}
              />
            </Popconfirm>
          )}
        </div>
        
        <Text type='tertiary' size='small' className='block mt-1'>
          {formatTime(record.timestamp)}
        </Text>
      </div>
    </div>
  );
};

export default HistoryModal;
