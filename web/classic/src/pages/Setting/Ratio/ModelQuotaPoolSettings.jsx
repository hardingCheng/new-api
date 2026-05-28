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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
  RadioGroup,
  Select,
  Space,
  Switch,
  Table,
  Tag,
} from '@douyinfe/semi-ui';
import { IconDelete, IconEdit, IconPlus, IconSave } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../../helpers/quota';
import { renderQuota } from '../../../helpers/render';

const SCOPE_GLOBAL = 'global';
const SCOPE_USER = 'user';
const METRIC_REQUESTS = 'requests';
const METRIC_TOTAL_TOKENS = 'total_tokens';
const METRIC_QUOTA = 'quota';

const METRIC_OPTIONS = [
  { value: METRIC_REQUESTS, label: '请求次数', unit: '次' },
  { value: METRIC_TOTAL_TOKENS, label: '总 Token', unit: 'tokens' },
  { value: METRIC_QUOTA, label: '花费金额', unit: '' },
];

const PERIOD_OPTIONS = [
  { value: 'minute', label: '每分钟' },
  { value: 'hour', label: '每小时' },
  { value: 'day', label: '每日' },
  { value: 'week', label: '每周' },
  { value: 'month', label: '每月' },
];

const emptyRule = {
  id: '',
  model: '',
  scope: SCOPE_GLOBAL,
  metric: METRIC_REQUESTS,
  user_id: 0,
  username: '',
  user_group: '',
  period: 'day',
  limit: 500,
  message: '',
  disabled: false,
};

const parseConfig = (raw) => {
  if (!raw || raw.trim() === '') {
    return [];
  }
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed?.rules) ? parsed.rules : [];
  } catch (error) {
    return [];
  }
};

const buildRawValue = (rules) => JSON.stringify({ rules }, null, 2);

const scopeText = (scope, t) =>
  scope === SCOPE_USER ? t('指定用户池') : t('全局共享池');

const periodText = (period, t) => {
  const item = PERIOD_OPTIONS.find((option) => option.value === period);
  return item ? t(item.label) : period;
};

const metricOption = (metric) =>
  METRIC_OPTIONS.find((option) => option.value === metric) || METRIC_OPTIONS[0];

const metricText = (metric, t) => t(metricOption(metric).label);

const metricUnitText = (metric, t) => t(metricOption(metric).unit);

const normalizeMetric = (metric) => {
  if (metric === 'prompt_tokens') {
    return METRIC_TOTAL_TOKENS;
  }
  return METRIC_OPTIONS.some((option) => option.value === metric)
    ? metric
    : METRIC_REQUESTS;
};

const formatLimitValue = (limit, metric, t) => {
  if (metric === METRIC_QUOTA) {
    return renderQuota(limit, 6);
  }
  return `${limit} ${metricUnitText(metric, t)}`;
};

const normalizeRule = (rule) => ({
  ...emptyRule,
  ...rule,
  id: rule.id || `rule-${Date.now()}`,
  model: String(rule.model || '').trim(),
  scope: rule.scope === SCOPE_USER ? SCOPE_USER : SCOPE_GLOBAL,
  metric: normalizeMetric(rule.metric),
  user_id: Number(rule.user_id) || 0,
  username: String(rule.username || '').trim(),
  user_group: String(rule.user_group || '').trim(),
  period: rule.period || 'day',
  limit: Number(rule.limit) || 0,
  message: String(rule.message || '').trim(),
  disabled: Boolean(rule.disabled),
});

export default function ModelQuotaPoolSettings({ options, refresh }) {
  const { t } = useTranslation();
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingId, setEditingId] = useState('');
  const [formState, setFormState] = useState(emptyRule);
  const [userOptions, setUserOptions] = useState([]);

  useEffect(() => {
    setRules(parseConfig(options.ModelQuotaPool).map(normalizeRule));
  }, [options.ModelQuotaPool]);

  const rawValue = useMemo(() => buildRawValue(rules), [rules]);

  const searchUsers = async (keyword = '') => {
    const params = new URLSearchParams({
      keyword,
      p: '0',
      page_size: '20',
    });
    const res = await API.get(`/api/user/search?${params.toString()}`);
    if (!res?.data?.success) {
      showError(res?.data?.message || t('搜索用户失败'));
      return;
    }
    setUserOptions(res.data.data?.items || []);
  };

  const openCreateModal = async () => {
    setEditingId('');
    setFormState({ ...emptyRule, id: `rule-${Date.now()}` });
    await searchUsers();
    setModalVisible(true);
  };

  const openEditModal = async (rule) => {
    setEditingId(rule.id);
    setFormState(normalizeRule(rule));
    await searchUsers(rule.username || String(rule.user_id || ''));
    setModalVisible(true);
  };

  const upsertRule = () => {
    const next = normalizeRule(formState);
    if (!next.model) {
      showError(t('请输入模型匹配规则'));
      return;
    }
    if (next.limit <= 0) {
      showError(t('限量值必须大于 0'));
      return;
    }
    if (next.scope === SCOPE_USER && next.user_id <= 0) {
      showError(t('请选择用户'));
      return;
    }

    setRules((previous) => {
      const filtered = previous.filter(
        (rule) => rule.id !== editingId && rule.id !== next.id,
      );
      return [...filtered, next].sort((a, b) =>
        `${a.model}-${a.scope}-${a.user_id}`.localeCompare(
          `${b.model}-${b.scope}-${b.user_id}`,
        ),
      );
    });
    setModalVisible(false);
  };

  const deleteRule = (id) => {
    setRules((previous) => previous.filter((rule) => rule.id !== id));
  };

  const saveRules = async () => {
    setLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'ModelQuotaPool',
        value: rawValue,
      });
      if (!res?.data?.success) {
        throw new Error(res?.data?.message || t('保存失败'));
      }
      showSuccess(t('保存成功'));
      await refresh();
    } catch (error) {
      showError(error.message || t('保存失败'));
    } finally {
      setLoading(false);
    }
  };

  const columns = [
    {
      title: t('状态'),
      width: 90,
      render: (_, record) => (
        <Tag color={record.disabled ? 'grey' : 'green'} shape='circle'>
          {record.disabled ? t('停用') : t('启用')}
        </Tag>
      ),
    },
    {
      title: t('模型匹配'),
      dataIndex: 'model',
      render: (text) => <Tag shape='circle'>{text}</Tag>,
    },
    {
      title: t('池类型'),
      width: 120,
      render: (_, record) => (
        <Tag
          color={record.scope === SCOPE_USER ? 'purple' : 'blue'}
          shape='circle'
        >
          {scopeText(record.scope, t)}
        </Tag>
      ),
    },
    {
      title: t('指定用户'),
      width: 220,
      render: (_, record) =>
        record.scope === SCOPE_USER
          ? `${record.user_id} / ${record.username || '-'} / ${record.user_group || '-'}`
          : t('所有用户'),
    },
    {
      title: t('限量维度'),
      width: 120,
      render: (_, record) => metricText(record.metric, t),
    },
    {
      title: t('周期'),
      width: 100,
      render: (_, record) => periodText(record.period, t),
    },
    {
      title: t('限量值'),
      width: 130,
      render: (_, record) => formatLimitValue(record.limit, record.metric, t),
    },
    {
      title: t('操作'),
      width: 180,
      render: (_, record) => (
        <Space>
          <Button
            size='small'
            theme='light'
            type='tertiary'
            icon={<IconEdit />}
            onClick={() => openEditModal(record)}
          >
            {t('编辑')}
          </Button>
          <Button
            size='small'
            theme='light'
            type='danger'
            icon={<IconDelete />}
            onClick={() => deleteRule(record.id)}
          >
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <Space vertical align='start' style={{ width: '100%' }}>
      <Card style={{ width: '100%' }}>
        <Space wrap>
          <Button icon={<IconPlus />} onClick={openCreateModal}>
            {t('新增规则')}
          </Button>
          <Button
            type='primary'
            icon={<IconSave />}
            loading={loading}
            onClick={saveRules}
          >
            {t('保存模型限量池')}
          </Button>
        </Space>
        <div className='mt-3 text-sm text-gray-500'>
          {t(
            '限制某些模型在指定周期内的请求次数、预估总 Token 或预估花费金额。全局共享池所有用户共同消耗；指定用户池只限制某个用户。超过后不会请求上游，也不会扣费。',
          )}
        </div>
      </Card>

      <Card bodyStyle={{ padding: 0 }} style={{ width: '100%' }}>
        <Table
          columns={columns}
          dataSource={rules}
          rowKey='id'
          pagination={false}
          empty={<Empty title={t('暂无模型限量池规则')} />}
        />
      </Card>

      <Modal
        title={editingId ? t('编辑模型限量池') : t('新增模型限量池')}
        visible={modalVisible}
        width={760}
        onOk={upsertRule}
        onCancel={() => setModalVisible(false)}
        okText={t('确认')}
        cancelText={t('取消')}
      >
        <Form layout='vertical'>
          <div className='mb-2 font-medium text-gray-700'>
            {t('模型匹配规则')}
          </div>
          <Input
            value={formState.model}
            placeholder='seedance-2.0-fast-480p 或 seedance-*'
            style={{ marginBottom: 16 }}
            onChange={(value) => setFormState({ ...formState, model: value })}
          />

          <div className='mb-2 font-medium text-gray-700'>{t('池类型')}</div>
          <RadioGroup
            type='button'
            value={formState.scope}
            onChange={(event) =>
              setFormState({
                ...formState,
                scope: event.target.value,
                user_id:
                  event.target.value === SCOPE_GLOBAL ? 0 : formState.user_id,
              })
            }
          >
            <Radio value={SCOPE_GLOBAL}>{t('全局共享池')}</Radio>
            <Radio value={SCOPE_USER}>{t('指定用户池')}</Radio>
          </RadioGroup>

          {formState.scope === SCOPE_USER ? (
            <>
              <div className='mb-2 mt-4 font-medium text-gray-700'>
                {t('指定用户')}
              </div>
              <Select
                filter
                remote
                showClear
                value={formState.user_id || undefined}
                placeholder={t('搜索并选择用户')}
                style={{ width: '100%', marginBottom: 16 }}
                onSearch={searchUsers}
                onChange={(value) => {
                  const selected = userOptions.find(
                    (user) => user.id === value,
                  );
                  setFormState({
                    ...formState,
                    user_id: value || 0,
                    username: selected?.username || '',
                    user_group: selected?.group || '',
                  });
                }}
              >
                {userOptions.map((user) => (
                  <Select.Option key={user.id} value={user.id}>
                    {user.id} / {user.username} / {user.group}
                  </Select.Option>
                ))}
                {formState.user_id &&
                !userOptions.some((user) => user.id === formState.user_id) ? (
                  <Select.Option value={formState.user_id}>
                    {formState.user_id} / {formState.username || '-'} /{' '}
                    {formState.user_group || '-'}
                  </Select.Option>
                ) : null}
              </Select>
            </>
          ) : null}

          <div className='mb-2 font-medium text-gray-700'>{t('限量维度')}</div>
          <RadioGroup
            type='button'
            value={formState.metric}
            onChange={(event) =>
              setFormState({
                ...formState,
                metric: event.target.value,
                limit:
                  event.target.value === METRIC_QUOTA &&
                  formState.metric !== METRIC_QUOTA
                    ? displayAmountToQuota(formState.limit)
                    : event.target.value !== METRIC_QUOTA &&
                        formState.metric === METRIC_QUOTA
                      ? Math.max(
                          1,
                          Math.round(quotaToDisplayAmount(formState.limit)),
                        )
                      : formState.limit,
              })
            }
          >
            {METRIC_OPTIONS.map((item) => (
              <Radio key={item.value} value={item.value}>
                {t(item.label)}
              </Radio>
            ))}
          </RadioGroup>

          <div className='mb-2 font-medium text-gray-700'>{t('周期')}</div>
          <Select
            value={formState.period}
            style={{ width: '100%', marginBottom: 16 }}
            onChange={(value) =>
              setFormState({ ...formState, period: value || 'day' })
            }
          >
            {PERIOD_OPTIONS.map((item) => (
              <Select.Option key={item.value} value={item.value}>
                {t(item.label)}
              </Select.Option>
            ))}
          </Select>

          <div className='mb-2 font-medium text-gray-700'>{t('限量值')}</div>
          <InputNumber
            value={
              formState.metric === METRIC_QUOTA
                ? quotaToDisplayAmount(formState.limit)
                : formState.limit
            }
            min={1}
            step={formState.metric === METRIC_QUOTA ? 0.01 : 1}
            suffix={
              formState.metric === METRIC_QUOTA
                ? undefined
                : metricUnitText(formState.metric, t)
            }
            style={{ width: '100%', marginBottom: 16 }}
            onChange={(value) =>
              setFormState({
                ...formState,
                limit:
                  formState.metric === METRIC_QUOTA
                    ? displayAmountToQuota(value)
                    : Number(value) || 0,
              })
            }
          />

          <div className='mb-2 font-medium text-gray-700'>
            {t('超过后提示')}
          </div>
          <Input
            value={formState.message}
            placeholder={t('模型当前周期限量已达上限')}
            style={{ marginBottom: 16 }}
            onChange={(value) => setFormState({ ...formState, message: value })}
          />

          <Space>
            <Switch
              checked={!formState.disabled}
              onChange={(checked) =>
                setFormState({ ...formState, disabled: !checked })
              }
            />
            <span>{formState.disabled ? t('停用') : t('启用')}</span>
          </Space>
        </Form>
      </Modal>
    </Space>
  );
}
