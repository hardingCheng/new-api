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
  InputNumber,
  Button,
  Typography,
  Toast,
  Popconfirm,
  Card,
} from '@douyinfe/semi-ui';
import { IconDeleteStroked } from '@douyinfe/semi-icons';
import {
  getCacheConfig,
  saveCacheConfig,
  clearAllCache,
  cleanupCache,
  DEFAULT_CACHE_CONFIG,
} from '../../utils/imageCache';

const { Text } = Typography;

const CacheSettings = ({ cacheStats }) => {
  const [config, setConfig] = useState(DEFAULT_CACHE_CONFIG);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const loadedConfig = getCacheConfig();
    setConfig(loadedConfig);
  }, []);

  const handleSave = async () => {
    setLoading(true);
    try {
      saveCacheConfig(config);
      await cleanupCache();
      Toast.success('设置已保存并应用清理规则');
    } catch (error) {
      Toast.error('保存设置失败');
    } finally {
      setLoading(false);
    }
  };

  const handleClearAll = async () => {
    setLoading(true);
    try {
      await clearAllCache();
      Toast.success('已清空所有缓存');
      window.location.reload();
    } catch (error) {
      Toast.error('清空缓存失败');
    } finally {
      setLoading(false);
    }
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

  return (
    <div className='p-6'>
      {/* 缓存统计 */}
      <Card
        title='缓存统计'
        headerExtraContent={
          <Popconfirm
            title='确定要清空所有缓存吗？'
            content='此操作不可恢复，将删除所有缓存的图片'
            onConfirm={handleClearAll}
          >
            <Button
              icon={<IconDeleteStroked />}
              type='danger'
              size='small'
              loading={loading}
            >
              清空缓存
            </Button>
          </Popconfirm>
        }
      >
        <div className='space-y-3'>
          <div className='flex justify-between items-center'>
            <Text type='secondary'>缓存图片数量</Text>
            <Text strong>{cacheStats?.count || 0} 张</Text>
          </div>
          <div className='flex justify-between items-center'>
            <Text type='secondary'>占用存储空间</Text>
            <Text strong>{formatSize(cacheStats?.totalSize || 0)}</Text>
          </div>
        </div>
      </Card>

      {/* 清理规则 */}
      <Card title='自动清理规则' className='mt-4'>
        <div className='space-y-4'>
          <div>
            <div className='flex items-center gap-2 mb-2'>
              <Text>保存时间</Text>
              <InputNumber
                suffix='天'
                min={1}
                max={365}
                value={formatDays(config.maxAge)}
                onChange={(value) => {
                  setConfig({
                    ...config,
                    maxAge: value * 24 * 60 * 60 * 1000,
                  });
                }}
                style={{ width: 200 }}
              />
            </div>
            <Text type='secondary' size='small'>
              超过此时间的图片将被自动清理（默认：{formatDays(DEFAULT_CACHE_CONFIG.maxAge)}天）
            </Text>
          </div>

          <div>
            <div className='flex items-center gap-2 mb-2'>
              <Text>最大数量</Text>
              <InputNumber
                suffix='张'
                min={10}
                max={1000}
                value={config.maxCount}
                onChange={(value) => {
                  setConfig({
                    ...config,
                    maxCount: value,
                  });
                }}
                style={{ width: 200 }}
              />
            </div>
            <Text type='secondary' size='small'>
              超过此数量时，最旧的图片将被清理（默认：{DEFAULT_CACHE_CONFIG.maxCount}张）
            </Text>
          </div>

          <div>
            <div className='flex items-center gap-2 mb-2'>
              <Text>最大存储</Text>
              <InputNumber
                suffix='MB'
                min={10}
                max={1000}
                value={Math.round(config.maxSize / (1024 * 1024))}
                onChange={(value) => {
                  setConfig({
                    ...config,
                    maxSize: value * 1024 * 1024,
                  });
                }}
                style={{ width: 200 }}
              />
            </div>
            <Text type='secondary' size='small'>
              超过此大小时，最旧的图片将被清理（默认：{Math.round(DEFAULT_CACHE_CONFIG.maxSize / (1024 * 1024))}MB）
            </Text>
          </div>

          <div className='flex justify-end gap-2 mt-4'>
            <Button
              onClick={() => {
                setConfig(DEFAULT_CACHE_CONFIG);
                Toast.info('已恢复默认设置');
              }}
            >
              恢复默认
            </Button>
            <Button
              theme='solid'
              type='primary'
              onClick={handleSave}
              loading={loading}
            >
              保存设置
            </Button>
          </div>
        </div>
      </Card>
    </div>
  );
};

export default CacheSettings;
