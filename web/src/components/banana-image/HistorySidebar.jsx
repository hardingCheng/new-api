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
import {
  Typography,
  Button,
  Empty,
  Popconfirm,
} from '@douyinfe/semi-ui';
import {
  IconClose,
  IconDelete,
  IconDeleteStroked,
} from '@douyinfe/semi-icons';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import 'dayjs/locale/zh-cn';

dayjs.extend(relativeTime);
dayjs.locale('zh-cn');

const { Title, Text } = Typography;

const HistorySidebar = ({
  records,
  onSelect,
  onDelete,
  onClear,
  onClose,
  isMobile,
}) => {
  const formatTime = (timestamp) => {
    return dayjs(timestamp).fromNow();
  };

  return (
    <div className='h-full flex flex-col'>
      {/* 标题栏 */}
      <div className='flex items-center justify-between p-4 border-b border-[var(--semi-color-border)]'>
        <Title heading={5} className='!mb-0'>
          历史记录
        </Title>
        <div className='flex items-center gap-2'>
          {records.length > 0 && (
            <Popconfirm
              title='确定要清空所有历史记录吗？'
              content='此操作不可恢复'
              onConfirm={onClear}
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
          {isMobile && onClose && (
            <Button
              icon={<IconClose />}
              theme='borderless'
              onClick={onClose}
            />
          )}
        </div>
      </div>

      {/* 历史列表 */}
      <div className='flex-1 overflow-y-auto'>
        {records.length === 0 ? (
          <div className='p-8'>
            <Empty
              image={<div className='text-4xl'>📜</div>}
              title='暂无历史记录'
              description='生成的图像会保存在这里'
            />
          </div>
        ) : (
          <div className='p-2 space-y-2'>
            {records.map((record) => (
              <HistoryItem
                key={record.id}
                record={record}
                onSelect={() => onSelect(record)}
                onDelete={() => onDelete(record.id)}
                formatTime={formatTime}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

const HistoryItem = ({ record, onSelect, onDelete, formatTime }) => {
  const thumbnailUrl = record.images?.[0]?.url;

  return (
    <div
      className='group relative flex gap-3 p-2 rounded-lg hover:bg-[var(--semi-color-fill-0)] cursor-pointer transition-colors'
      onClick={onSelect}
    >
      {/* 缩略图 */}
      <div className='flex-shrink-0 w-16 h-16 rounded-md overflow-hidden bg-[var(--semi-color-fill-1)]'>
        {thumbnailUrl ? (
          <img
            src={thumbnailUrl}
            alt='Thumbnail'
            className='w-full h-full object-cover'
          />
        ) : (
          <div className='w-full h-full flex items-center justify-center text-2xl'>
            🖼️
          </div>
        )}
      </div>

      {/* 信息 */}
      <div className='flex-1 min-w-0'>
        <Text
          ellipsis={{ showTooltip: true }}
          className='block text-sm font-medium'
        >
          {record.prompt || '无提示词'}
        </Text>
        <div className='flex items-center gap-2 mt-1'>
          <Text type='tertiary' size='small'>
            {record.model?.split('/').pop() || '未知模型'}
          </Text>
          <Text type='tertiary' size='small'>
            •
          </Text>
          <Text type='tertiary' size='small'>
            {formatTime(record.timestamp)}
          </Text>
        </div>
        {record.params && (
          <Text type='tertiary' size='small' className='block mt-1'>
            {record.params.width}×{record.params.height}
          </Text>
        )}
      </div>

      {/* 删除按钮 */}
      <Button
        icon={<IconDelete />}
        theme='borderless'
        type='danger'
        size='small'
        className='opacity-0 group-hover:opacity-100 transition-opacity absolute top-2 right-2'
        onClick={(e) => {
          e.stopPropagation();
          onDelete();
        }}
      />
    </div>
  );
};

export default HistorySidebar;
