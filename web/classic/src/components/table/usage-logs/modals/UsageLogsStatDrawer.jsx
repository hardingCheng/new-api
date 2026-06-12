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

import React, { useState, useEffect, useCallback } from 'react';
import {
  SideSheet,
  Button,
  DatePicker,
  Tabs,
  TabPane,
  Table,
  Tag,
  Typography,
  Space,
} from '@douyinfe/semi-ui';
import { IconHistogram } from '@douyinfe/semi-icons';
import { API, showError, renderQuota } from '../../../../helpers';

const { Text } = Typography;

const DIMENSIONS = [
  { key: 'channel', label: '渠道' },
  { key: 'model', label: '模型' },
  { key: 'user', label: '用户' },
];

const defaultRange = () => {
  const end = new Date();
  const start = new Date();
  start.setHours(0, 0, 0, 0);
  return [start, end];
};

const UsageLogsStatDrawer = ({ t }) => {
  const [visible, setVisible] = useState(false);
  const [dimension, setDimension] = useState('channel');
  const [range, setRange] = useState(defaultRange());
  const [loading, setLoading] = useState(false);
  const [rows, setRows] = useState([]);

  const fetchData = useCallback(async () => {
    if (!Array.isArray(range) || range.length !== 2 || !range[0] || !range[1]) {
      return;
    }
    setLoading(true);
    try {
      const startTs = Math.floor(new Date(range[0]).getTime() / 1000);
      const endTs = Math.floor(new Date(range[1]).getTime() / 1000);
      const url = `/api/log/stat/breakdown?dimension=${dimension}&start_timestamp=${startTs}&end_timestamp=${endTs}`;
      const res = await API.get(url);
      const { success, message, data } = res.data;
      if (success) {
        setRows(Array.isArray(data) ? data : []);
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  }, [dimension, range]);

  useEffect(() => {
    if (visible) {
      fetchData();
    }
  }, [visible, dimension, fetchData]);

  const rowName = (record) => {
    if (dimension === 'channel') {
      return record.name && record.name !== ''
        ? record.name
        : `#${record.channel_id}`;
    }
    return record.name && record.name !== '' ? record.name : '-';
  };

  // 收入 = 消耗 - 退款
  const incomeOf = (r) => (r.consume_quota || 0) - (r.refund_quota || 0);

  const columns = [
    ...(dimension === 'channel'
      ? [
          {
            title: t('渠道名称'),
            dataIndex: 'name',
            render: (_, record) => (
              <Text strong>
                {record.name && record.name !== '' ? record.name : '-'}
              </Text>
            ),
          },
          {
            title: t('渠道号'),
            dataIndex: 'channel_id',
            render: (text) => <Text>#{text}</Text>,
          },
        ]
      : [
          {
            title: dimension === 'model' ? t('模型') : t('用户'),
            dataIndex: 'name',
            render: (_, record) => <Text strong>{rowName(record)}</Text>,
          },
        ]),
    {
      title: t('消耗额度'),
      dataIndex: 'consume_quota',
      sorter: (a, b) => a.consume_quota - b.consume_quota,
      render: (text) => (
        <Tag color='blue' type='light' shape='circle'>
          {renderQuota(text || 0)}
        </Tag>
      ),
    },
    {
      title: t('退款额度'),
      dataIndex: 'refund_quota',
      sorter: (a, b) => a.refund_quota - b.refund_quota,
      render: (text) =>
        text > 0 ? (
          <Tag color='green' type='light' shape='circle'>
            {renderQuota(text)}
          </Tag>
        ) : (
          '-'
        ),
    },
    {
      title: t('收入'),
      dataIndex: 'income',
      sorter: (a, b) => incomeOf(a) - incomeOf(b),
      render: (_, record) => (
        <Tag color='orange' type='light' shape='circle'>
          {renderQuota(incomeOf(record))}
        </Tag>
      ),
    },
    {
      title: t('次数'),
      dataIndex: 'count',
      sorter: (a, b) => a.count - b.count,
    },
  ];

  const totalConsume = rows.reduce((s, r) => s + (r.consume_quota || 0), 0);
  const totalRefund = rows.reduce((s, r) => s + (r.refund_quota || 0), 0);
  const totalIncome = totalConsume - totalRefund;

  return (
    <>
      <Button
        type='tertiary'
        size='small'
        icon={<IconHistogram />}
        onClick={() => setVisible(true)}
      >
        {t('统计')}
      </Button>
      <SideSheet
        title={t('额度统计')}
        visible={visible}
        onCancel={() => setVisible(false)}
        width={720}
        bodyStyle={{ padding: 16 }}
      >
        <Space style={{ marginBottom: 12 }}>
          <DatePicker
            type='dateTimeRange'
            value={range}
            onChange={(v) => setRange(v)}
            style={{ width: 400 }}
          />
          <Button theme='solid' loading={loading} onClick={fetchData}>
            {t('查询')}
          </Button>
        </Space>

        <Space style={{ marginBottom: 12 }}>
          <Tag color='blue' type='light' shape='circle'>
            {t('总消耗')}: {renderQuota(totalConsume)}
          </Tag>
          <Tag color='green' type='light' shape='circle'>
            {t('总退款')}: {renderQuota(totalRefund)}
          </Tag>
          <Tag color='orange' type='light' shape='circle'>
            {t('总收入')}: {renderQuota(totalIncome)}
          </Tag>
        </Space>

        <Tabs
          type='line'
          activeKey={dimension}
          onChange={(key) => setDimension(key)}
        >
          {DIMENSIONS.map((d) => (
            <TabPane tab={t(d.label)} itemKey={d.key} key={d.key}>
              <Table
                columns={columns}
                dataSource={rows}
                loading={loading}
                pagination={{ pageSize: 20 }}
                rowKey={(record, idx) =>
                  `${dimension}-${record.channel_id}-${record.name}-${idx}`
                }
                size='small'
                empty={t('暂无数据')}
              />
            </TabPane>
          ))}
        </Tabs>
      </SideSheet>
    </>
  );
};

export default UsageLogsStatDrawer;
