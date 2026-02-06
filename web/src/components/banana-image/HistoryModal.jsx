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
}) => {
  const [activeTab, setActiveTab] = useState('history');
  const [searchText, setSearchText] = useState('');
  const [cacheConfig, setCacheConfig] = useState(null);

  // 加载缓存配置
  useEffect(() => {
    if (visible) {
      const config = getCacheConfig();
      setCacheConfig(config);
    }
  }, [visible]);

  // 根据搜索文本过滤记录
  const filteredRecords = useMemo(() => {
    if (!searchText.trim()) {
      return records;
    }
    const lowerSearchText = searchText.toLowerCase();
    return records.filter((record) => {
      const prompt = record.prompt || '';
      return prompt.toLowerCase().includes(lowerSearchText);
    });
  }, [records, searchText]);

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
      width={800}
      bodyStyle={{ padding: 0, height: '70vh' }}
      title={
        <div className='flex items-center justify-between pr-4'>
          <span>历史记录</span>
          {activeTab === 'history' && records.length > 0 && (
            <Popconfirm
              title='确定要清空所有历史记录吗？'
              content='此操作不可恢复'
              onConfirm={() => {
                onClear();
                Toast.success('已清空历史记录');
              }}
            >
              <Button
                icon={<IconDeleteStroked />}
                theme='borderless'
                type='danger'
                size='small'
              >
                清空
              </Button>
            </Popconfirm>
          )}
        </div>
      }
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
              📜 历史记录 {records.length > 0 && `(${records.length})`}
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

            {records.length === 0 ? (
              <div className='p-8'>
                <Empty
                  image={<div className='text-4xl'>📜</div>}
                  title='暂无历史记录'
                  description='生成的图像会保存在这里'
                />
              </div>
            ) : (
              <>
                {/* 搜索框 */}
                <div className='p-4 pb-2 sticky top-0 bg-[var(--semi-color-bg-0)] z-10'>
                  <Input
                    prefix={<IconSearch />}
                    placeholder='搜索提示词...'
                    value={searchText}
                    onChange={setSearchText}
                    showClear
                  />
                </div>

                {/* 记录列表 */}
                {filteredRecords.length === 0 ? (
                  <div className='p-8'>
                    <Empty
                      image={<div className='text-4xl'>🔍</div>}
                      title='未找到匹配的记录'
                      description='尝试使用其他关键词搜索'
                    />
                  </div>
                ) : (
                  <div className='p-4 pt-2 grid grid-cols-1 md:grid-cols-2 gap-4'>
                    {filteredRecords.map((record) => (
                      <HistoryCard
                        key={record.id}
                        record={record}
                        onSelect={() => {
                          onSelect(record);
                          onClose();
                        }}
                        onDelete={() => onDelete(record.id)}
                        onDownload={handleDownloadImage}
                        formatTime={formatTime}
                      />
                    ))}
                  </div>
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

const HistoryCard = ({ record, onSelect, onDelete, onDownload, formatTime }) => {
  const thumbnailUrl = record.images?.[0]?.url;
  const imageCount = record.images?.length || 0;

  return (
    <div className='group relative rounded-lg border border-[var(--semi-color-border)] hover:border-[var(--semi-color-primary)] transition-all overflow-hidden bg-[var(--semi-color-bg-1)]'>
      {/* 缩略图 */}
      <div
        className='relative w-full h-40 bg-[var(--semi-color-fill-1)] cursor-pointer'
        onClick={onSelect}
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
        
        {/* 图片数量标签 */}
        {imageCount > 1 && (
          <div className='absolute top-2 right-2 bg-black/70 text-white px-2 py-1 rounded text-xs'>
            {imageCount} 张
          </div>
        )}

        {/* 悬浮操作按钮 */}
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
      </div>

      {/* 信息区域 */}
      <div className='p-3'>
        <Text
          ellipsis={{ showTooltip: true, rows: 2 }}
          className='block text-sm font-medium mb-2'
        >
          {record.prompt || '无提示词'}
        </Text>
        
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
        </div>
        
        <Text type='tertiary' size='small' className='block mt-1'>
          {formatTime(record.timestamp)}
        </Text>
      </div>
    </div>
  );
};

export default HistoryModal;
