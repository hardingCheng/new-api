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

import React, { useMemo, useState } from 'react';
import {
  Button,
  Progress,
  SideSheet,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconChevronRight, IconServer } from '@douyinfe/semi-icons';
import { renderQuota } from '../../../helpers/render';

const { Text } = Typography;

const periodText = (period, t) => {
  switch (period) {
    case 'minute':
      return t('每分钟');
    case 'hour':
      return t('每小时');
    case 'day':
      return t('每日');
    case 'week':
      return t('每周');
    case 'month':
      return t('每月');
    default:
      return period || '-';
  }
};

const scopeText = (scope, t) =>
  scope === 'user' ? t('用户专属池') : t('全局共享池');

const metricText = (metric, t) => {
  switch (metric) {
    case 'total_tokens':
    case 'prompt_tokens':
      return t('总 Token');
    case 'quota':
      return t('花费金额');
    case 'requests':
    default:
      return t('请求次数');
  }
};

const metricUnitText = (metric, t) => {
  switch (metric) {
    case 'total_tokens':
    case 'prompt_tokens':
      return 'tokens';
    case 'quota':
      return t('额度');
    case 'requests':
    default:
      return t('次');
  }
};

const normalizeMetric = (metric) =>
  metric === 'prompt_tokens' ? 'total_tokens' : metric;

const formatMetricValue = (value, metric, t) => {
  const normalizedMetric = normalizeMetric(metric);
  if (normalizedMetric === 'quota') {
    return renderQuota(value, 6);
  }
  return `${value} ${metricUnitText(normalizedMetric, t)}`;
};

const progressColor = (percent) => {
  if (percent >= 90) return 'red';
  if (percent >= 70) return 'orange';
  return 'green';
};

const ModelQuotaPoolSummary = ({ quotaPools, t }) => {
  const [drawerVisible, setDrawerVisible] = useState(false);
  const summary = useMemo(() => {
    const items = Array.isArray(quotaPools) ? quotaPools : [];
    let minRemainingPercent = 100;
    let tightCount = 0;
    for (const item of items) {
      const limit = Number(item.limit) || 0;
      const remaining = Number(item.remaining) || 0;
      const remainingPercent =
        limit > 0 ? Math.max(0, Math.round((remaining / limit) * 100)) : 100;
      minRemainingPercent = Math.min(minRemainingPercent, remainingPercent);
      if (remainingPercent <= 20) {
        tightCount += 1;
      }
    }
    return {
      count: items.length,
      tightCount,
      minRemainingPercent: items.length > 0 ? minRemainingPercent : 100,
    };
  }, [quotaPools]);

  if (!Array.isArray(quotaPools) || quotaPools.length === 0) {
    return null;
  }

  return (
    <>
      <div className='mb-2 max-w-full overflow-x-auto'>
        <div className='inline-flex min-w-max items-center gap-2 rounded-md border border-gray-200 bg-white px-2 py-1 shadow-sm'>
          <div className='flex h-6 w-6 shrink-0 items-center justify-center rounded bg-blue-50 text-blue-600'>
            <IconServer />
          </div>
          <span className='text-sm font-medium text-gray-900'>
            {t('模型限量池')}
          </span>
          <Tag
            shape='circle'
            color={summary.tightCount > 0 ? 'orange' : 'green'}
          >
            {summary.count} {t('个池子')}
          </Tag>
          {summary.tightCount > 0 ? (
            <Tag shape='circle' color='red'>
              {summary.tightCount} {t('个紧张')}
            </Tag>
          ) : null}
          <Text type='tertiary' size='small'>
            {t('最低剩余')} {summary.minRemainingPercent}%
          </Text>
          <Button
            theme='borderless'
            type='tertiary'
            size='small'
            icon={<IconChevronRight />}
            iconPosition='right'
            onClick={() => setDrawerVisible(true)}
          >
            {t('查看明细')}
          </Button>
        </div>
      </div>

      <SideSheet
        title={t('模型限量池明细')}
        visible={drawerVisible}
        width={720}
        onCancel={() => setDrawerVisible(false)}
        footer={null}
      >
        <div className='space-y-2'>
          {quotaPools.map((item, index) => {
            const rule = item.rule || {};
            const limit = Number(item.limit) || 0;
            const used = Number(item.used) || 0;
            const remaining = Number(item.remaining) || 0;
            const metric = normalizeMetric(
              item.metric || rule.metric || 'requests',
            );
            const percent =
              limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0;
            return (
              <div
                key={`${rule.id || rule.model}-${item.scope}-${item.period_key}-${index}`}
                className='rounded-md border border-gray-100 bg-gray-50 px-3 py-2'
              >
                <div className='flex items-center gap-3'>
                  <div className='min-w-0 flex-1'>
                    <div className='flex items-center gap-2'>
                      <span className='truncate text-sm font-medium text-gray-900'>
                        {rule.model}
                      </span>
                      <Tag
                        color={item.scope === 'user' ? 'purple' : 'blue'}
                        shape='circle'
                        className='shrink-0'
                      >
                        {scopeText(item.scope, t)}
                      </Tag>
                      <Tag shape='circle' className='shrink-0'>
                        {metricText(metric, t)}
                      </Tag>
                    </div>
                    <div className='mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500'>
                      <span>
                        {periodText(rule.period, t)} / {item.period_key}
                      </span>
                      {item.scope === 'user' ? (
                        <span>
                          {rule.user_id} / {rule.username || '-'} /{' '}
                          {rule.user_group || '-'}
                        </span>
                      ) : null}
                    </div>
                  </div>
                  <div className='w-[180px] shrink-0'>
                    <div className='mb-1 flex items-center justify-between text-xs'>
                      <span className='font-medium text-gray-900'>
                        {formatMetricValue(remaining, metric, t)}
                      </span>
                      <span className='text-gray-500'>{percent}%</span>
                    </div>
                    <Progress
                      percent={percent}
                      stroke={progressColor(percent)}
                      showInfo={false}
                      size='small'
                    />
                  </div>
                  <div className='w-[88px] shrink-0 text-right text-xs text-gray-500'>
                    {formatMetricValue(used, metric, t)} /{' '}
                    {formatMetricValue(limit, metric, t)}
                  </div>
                  <Tag
                    color={
                      percent >= 100
                        ? 'red'
                        : percent >= 70
                          ? 'orange'
                          : 'green'
                    }
                    shape='circle'
                    className='shrink-0'
                  >
                    {percent >= 100 ? '100%' : `${percent}%`}
                  </Tag>
                </div>
              </div>
            );
          })}
        </div>
      </SideSheet>
    </>
  );
};

export default ModelQuotaPoolSummary;
