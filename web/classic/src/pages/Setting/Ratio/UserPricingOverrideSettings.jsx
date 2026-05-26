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
  Select,
  Space,
  Spin,
  Table,
  Tag,
} from '@douyinfe/semi-ui';
import { IconDelete, IconEdit, IconPlus, IconSave } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

const TYPE_RATIO = 'ratio';
const TYPE_MODEL_PRICE = 'model_price';
const TYPE_MODEL_RATIO = 'model_ratio';
const SCENARIO_ALL_DISCOUNT = 'all_discount';
const SCENARIO_GROUP_DISCOUNT = 'group_discount';
const SCENARIO_MODEL_FIXED_PRICE = 'model_fixed_price';
const SCENARIO_MODEL_RATIO = 'model_ratio';

const parseConfig = (raw) => {
  if (!raw || raw.trim() === '') {
    return { rules: [] };
  }
  try {
    const parsed = JSON.parse(raw);
    return {
      rules: Array.isArray(parsed?.rules) ? parsed.rules : [],
    };
  } catch {
    return { rules: [] };
  }
};

const normalizeRules = (rules) =>
  rules.map((rule, index) => ({
    id: `${rule.user_id || 'user'}-${index}-${Date.now()}`,
    user_id: rule.user_id || 0,
    username: rule.username || '',
    user_group: rule.user_group || '',
    group_pattern: rule.group_pattern || '',
    model_pattern: rule.model_pattern || '',
    type: rule.type || TYPE_RATIO,
    value: rule.value ?? 1,
    disabled: Boolean(rule.disabled),
  }));

const buildRawValue = (rules) =>
  JSON.stringify(
    {
      rules: rules
        .map((rule) => ({
          user_id: Number(rule.user_id) || 0,
          username: String(rule.username || '').trim(),
          user_group: String(rule.user_group || '').trim(),
          group_pattern: String(rule.group_pattern || '').trim(),
          model_pattern: String(rule.model_pattern || '').trim(),
          type: rule.type,
          value: Number(rule.value) || 0,
          disabled: Boolean(rule.disabled),
        }))
        .filter((rule) => rule.user_id > 0 && rule.type),
    },
    null,
    2,
  );

const typeText = (type, t) => {
  if (type === TYPE_MODEL_PRICE) return t('固定单价');
  if (type === TYPE_MODEL_RATIO) return t('按量倍率');
  return t('整体倍率');
};

const matchText = (value, fallback, t) =>
  value ? <Tag shape='circle'>{value}</Tag> : <Tag shape='circle' color='grey'>{t(fallback)}</Tag>;

const emptyRule = {
  user_id: 0,
  username: '',
  user_group: '',
  group_pattern: '',
  model_pattern: '',
  type: TYPE_RATIO,
  value: 1,
  disabled: false,
};

const inferScenario = (rule) => {
  if (rule.type === TYPE_MODEL_PRICE) return SCENARIO_MODEL_FIXED_PRICE;
  if (rule.type === TYPE_MODEL_RATIO) return SCENARIO_MODEL_RATIO;
  if (rule.group_pattern) return SCENARIO_GROUP_DISCOUNT;
  return SCENARIO_ALL_DISCOUNT;
};

const applyScenario = (rule, scenario) => {
  if (scenario === SCENARIO_MODEL_FIXED_PRICE) {
    return { ...rule, type: TYPE_MODEL_PRICE };
  }
  if (scenario === SCENARIO_MODEL_RATIO) {
    return { ...rule, type: TYPE_MODEL_RATIO };
  }
  if (scenario === SCENARIO_ALL_DISCOUNT) {
    return { ...rule, type: TYPE_RATIO, group_pattern: '', model_pattern: '' };
  }
  return { ...rule, type: TYPE_RATIO, model_pattern: '' };
};

const scenarioOptions = (t) => [
  { value: SCENARIO_GROUP_DISCOUNT, label: t('给用户某个分组打折') },
  { value: SCENARIO_MODEL_FIXED_PRICE, label: t('给用户某个模型设置固定单价') },
  { value: SCENARIO_ALL_DISCOUNT, label: t('给用户全部使用打折') },
  { value: SCENARIO_MODEL_RATIO, label: t('给用户某个按量模型设置倍率') },
];

const formatScope = (group, model, t) => `${group || t('全部分组')} / ${model || t('全部模型')}`;

const ruleSummary = (rule, t) => {
  const user = rule.username ? `${rule.user_id} / ${rule.username}` : `${rule.user_id || '-'}`;
  const scope = formatScope(rule.group_pattern, rule.model_pattern, t);
  if (rule.type === TYPE_MODEL_PRICE) {
    return t('用户价格规则预览：{{user}} 使用 {{scope}} 时，按固定单价 {{value}} 计费。', {
      user,
      scope,
      value: rule.value || 0,
    });
  }
  if (rule.type === TYPE_MODEL_RATIO) {
    return t('用户价格规则预览：{{user}} 使用 {{scope}} 时，按量模型倍率使用 {{value}}。', {
      user,
      scope,
      value: rule.value || 0,
    });
  }
  return t('用户价格规则预览：{{user}} 使用 {{scope}} 时，整体倍率使用 {{value}}。', {
    user,
    scope,
    value: rule.value || 0,
  });
};

export default function UserPricingOverrideSettings({ options, refresh }) {
  const { t } = useTranslation();
  const [rules, setRules] = useState([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingRuleId, setEditingRuleId] = useState('');
  const [saving, setSaving] = useState(false);
  const [ruleForm, setRuleForm] = useState(emptyRule);
  const [scenario, setScenario] = useState(SCENARIO_GROUP_DISCOUNT);
  const [userOptions, setUserOptions] = useState([]);
  const [userContext, setUserContext] = useState(null);
  const [contextLoading, setContextLoading] = useState(false);

  useEffect(() => {
    const parsed = parseConfig(options.UserPricingOverride);
    setRules(normalizeRules(parsed.rules));
  }, [options.UserPricingOverride]);

  const rawValue = useMemo(() => buildRawValue(rules), [rules]);

  const searchUsers = async (keyword = '') => {
    const params = new URLSearchParams({
      keyword,
      group: '',
      p: '1',
      page_size: '20',
    });
    const res = await API.get(`/api/user/search?${params.toString()}`);
    if (!res?.data?.success) {
      showError(res?.data?.message || t('搜索用户失败'));
      return;
    }
    const users = res.data.data?.items || [];
    setUserOptions(users);
  };

  const loadUserContext = async (userId) => {
    if (!userId) {
      setUserContext(null);
      return;
    }
    setContextLoading(true);
    try {
      const res = await API.get(`/api/user/${userId}/pricing_context`);
      if (!res?.data?.success) {
        throw new Error(res?.data?.message || t('获取用户价格上下文失败'));
      }
      const context = res.data.data;
      setUserContext(context);
      setRuleForm((previous) => ({
        ...previous,
        user_id: context.user?.id || previous.user_id,
        username: context.user?.username || previous.username,
        user_group: context.user?.group || previous.user_group,
      }));
    } catch (error) {
      showError(error.message || t('获取用户价格上下文失败'));
      setUserContext(null);
    } finally {
      setContextLoading(false);
    }
  };

  const openRuleModal = async (rule = null) => {
    setEditingRuleId(rule?.id || '');
    setRuleForm(rule || emptyRule);
    setScenario(inferScenario(rule || emptyRule));
    setUserContext(null);
    if (rule?.user_id) {
      await loadUserContext(rule.user_id);
    } else {
      await searchUsers();
    }
    setModalVisible(true);
  };

  const saveRule = () => {
    if (!Number(ruleForm.user_id)) {
      showError(t('请填写用户 ID'));
      return;
    }
    const nextRule = {
      ...ruleForm,
      id: editingRuleId || `rule-${Date.now()}`,
      user_id: Number(ruleForm.user_id),
      username: String(ruleForm.username || '').trim(),
      user_group: String(ruleForm.user_group || '').trim(),
      group_pattern: String(ruleForm.group_pattern || '').trim(),
      model_pattern: String(ruleForm.model_pattern || '').trim(),
      value: Number(ruleForm.value) || 0,
    };
    setRules((previous) =>
      editingRuleId
        ? previous.map((item) => (item.id === editingRuleId ? nextRule : item))
        : [...previous, nextRule],
    );
    setModalVisible(false);
  };

  const saveAll = async () => {
    setSaving(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'UserPricingOverride',
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
      setSaving(false);
    }
  };

  const columns = [
    {
      title: t('用户'),
      width: 240,
      render: (_, record) => (
        <div>
          <div>{record.user_id}{record.username ? ` / ${record.username}` : ''}</div>
          {record.user_group ? <Tag size='small' shape='circle'>{record.user_group}</Tag> : null}
        </div>
      ),
    },
    {
      title: t('适用分组'),
      render: (_, record) => matchText(record.group_pattern, '全部分组', t),
    },
    {
      title: t('适用模型'),
      render: (_, record) => matchText(record.model_pattern, '全部模型', t),
    },
    {
      title: t('价格动作'),
      render: (_, record) => (
        <Tag shape='circle' color={record.type === TYPE_MODEL_PRICE ? 'teal' : 'blue'}>
          {typeText(record.type, t)}：{record.value}
        </Tag>
      ),
    },
    {
      title: t('规则效果'),
      render: (_, record) => (
        <div style={{ whiteSpace: 'normal', wordBreak: 'break-word' }}>
          {ruleSummary(record, t)}
        </div>
      ),
    },
    {
      title: t('状态'),
      width: 100,
      render: (_, record) => (
        <Tag shape='circle' color={record.disabled ? 'red' : 'green'}>
          {record.disabled ? t('已禁用') : t('已启用')}
        </Tag>
      ),
    },
    {
      title: t('操作'),
      width: 150,
      render: (_, record) => (
        <Space>
          <Button size='small' icon={<IconEdit />} onClick={() => openRuleModal(record)} />
          <Button
            size='small'
            type='danger'
            icon={<IconDelete />}
            onClick={() => setRules((previous) => previous.filter((item) => item.id !== record.id))}
          />
        </Space>
      ),
    },
  ];

  return (
    <Space vertical align='start' style={{ width: '100%' }}>
      <Card style={{ width: '100%' }}>
        <Space wrap>
          <Button icon={<IconPlus />} onClick={() => openRuleModal()}>
            {t('新增用户价格规则')}
          </Button>
          <Button type='primary' icon={<IconSave />} loading={saving} onClick={saveAll}>
            {t('保存用户价格覆盖')}
          </Button>
        </Space>
        <div className='mt-3 text-sm text-gray-500'>
          {t('按运营场景新增规则：先选用户，再选择要打折的分组或要单独定价的模型。')}
        </div>
      </Card>

      <Card title={t('用户价格规则')} bodyStyle={{ padding: 0 }} style={{ width: '100%' }}>
        <Table
          columns={columns}
          dataSource={rules}
          rowKey='id'
          pagination={false}
          empty={<Empty title={t('暂无用户价格规则')} />}
        />
      </Card>

      <Modal
        title={editingRuleId ? t('编辑用户价格规则') : t('新增用户价格规则')}
        visible={modalVisible}
        onOk={saveRule}
        onCancel={() => setModalVisible(false)}
      >
        <Spin spinning={contextLoading}>
          <Form layout='vertical'>
            <div className='mb-2 font-medium text-gray-700'>{t('我要做什么')}</div>
            <Select
              value={scenario}
              style={{ width: '100%', marginBottom: 16 }}
              onChange={(value) => {
                setScenario(value);
                setRuleForm(applyScenario(ruleForm, value));
              }}
            >
              {scenarioOptions(t).map((item) => (
                <Select.Option key={item.value} value={item.value}>
                  {item.label}
                </Select.Option>
              ))}
            </Select>
            <div className='mb-2 font-medium text-gray-700'>{t('用户')}</div>
            <Select
              filter
              remote
              showClear
              value={ruleForm.user_id || undefined}
              placeholder={t('搜索并选择用户')}
              style={{ width: '100%', marginBottom: 12 }}
              onSearch={searchUsers}
              onChange={(value) => {
                const selected = userOptions.find((user) => user.id === value);
                setRuleForm({
                  ...ruleForm,
                  user_id: value || 0,
                  username: selected?.username || '',
                  user_group: selected?.group || '',
                  group_pattern: '',
                  model_pattern: '',
                });
                if (value) {
                  loadUserContext(value);
                } else {
                  setUserContext(null);
                }
              }}
            >
              {userOptions.map((user) => (
                <Select.Option key={user.id} value={user.id}>
                  {user.id} / {user.username} / {user.group}
                </Select.Option>
              ))}
              {ruleForm.user_id && !userOptions.some((user) => user.id === ruleForm.user_id) ? (
                <Select.Option value={ruleForm.user_id}>
                  {ruleForm.user_id} / {ruleForm.username || '-'} / {ruleForm.user_group || '-'}
                </Select.Option>
              ) : null}
            </Select>
            {userContext?.user ? (
              <div className='mb-3 text-sm text-gray-500'>
                {t('已选择')}：{userContext.user.id} / {userContext.user.username} / {userContext.user.group}
              </div>
            ) : null}
            {scenario !== SCENARIO_ALL_DISCOUNT ? (
              <>
                <div className='mb-2 font-medium text-gray-700'>{t('适用分组')}</div>
                <Select
                  filter
                  showClear
                  value={ruleForm.group_pattern || undefined}
                  placeholder={t('全部分组')}
                  style={{ width: '100%', marginBottom: 16 }}
                  onChange={(value) => setRuleForm({ ...ruleForm, group_pattern: value || '', model_pattern: '' })}
                >
                  {(userContext?.groups || []).map((group) => (
                    <Select.Option key={group.name} value={group.name}>
                      {group.name}{group.desc ? ` / ${group.desc}` : ''} / {group.models?.length || 0} {t('个模型')}
                    </Select.Option>
                  ))}
                </Select>
              </>
            ) : null}
            {scenario === SCENARIO_MODEL_FIXED_PRICE || scenario === SCENARIO_MODEL_RATIO ? (
              <>
                <div className='mb-2 font-medium text-gray-700'>{t('适用模型')}</div>
                <Select
                  filter
                  showClear
                  value={ruleForm.model_pattern || undefined}
                  placeholder={t('全部模型')}
                  style={{ width: '100%', marginBottom: 16 }}
                  onChange={(value) => setRuleForm({ ...ruleForm, model_pattern: value || '' })}
                >
                  {((ruleForm.group_pattern
                    ? userContext?.groups?.find((group) => group.name === ruleForm.group_pattern)?.models
                    : userContext?.models) || []).map((modelName) => (
                    <Select.Option key={modelName} value={modelName}>
                      {modelName}
                    </Select.Option>
                  ))}
                </Select>
                <div className='mb-2 font-medium text-gray-700'>{t('自定义模型通配符')}</div>
                <Input
                  value={ruleForm.model_pattern}
                  placeholder='seedance-*'
                  style={{ marginBottom: 16 }}
                  onChange={(value) => setRuleForm({ ...ruleForm, model_pattern: value })}
                />
              </>
            ) : null}
            <div className='mb-2 font-medium text-gray-700'>
              {ruleForm.type === TYPE_MODEL_PRICE ? t('固定单价') : t('倍率数值')}
            </div>
            <InputNumber
              value={ruleForm.value}
              min={0}
              step={0.1}
              style={{ width: '100%' }}
              onChange={(value) => setRuleForm({ ...ruleForm, value: value || 0 })}
            />
            <div className='mt-3 text-sm text-gray-500'>
              {ruleForm.type === TYPE_MODEL_PRICE
                ? t('固定单价会直接替换模型原来的固定价格。')
                : t('倍率填写 0.8 表示八折，1 表示原价，0 表示免费。')}
            </div>
            <Card className='mt-4' bodyStyle={{ padding: 12 }}>
              {ruleSummary(ruleForm, t)}
            </Card>
          </Form>
        </Spin>
      </Modal>
    </Space>
  );
}
