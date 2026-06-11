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
  Switch,
  Table,
  Tag,
} from '@douyinfe/semi-ui';
import { IconDelete, IconEdit, IconPlus, IconSave } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

const TYPE_RATIO = 'ratio';
const TYPE_MODEL_PRICE = 'model_price';
const TYPE_MODEL_RATIO = 'model_ratio';
// 参考视频秒数定价：仅作用于参考视频那部分秒数。
const TYPE_VIDEO_REF_FACTOR = 'video_ref_factor';
const TYPE_VIDEO_REF_PRICE = 'video_ref_price';
const TYPE_VIDEO_REF_FLAT = 'video_ref_flat';
const TYPE_VIDEO_REF_CAP = 'video_ref_cap';
const VIDEO_REF_TYPES = [TYPE_VIDEO_REF_FACTOR, TYPE_VIDEO_REF_PRICE, TYPE_VIDEO_REF_FLAT, TYPE_VIDEO_REF_CAP];
const isVideoRefType = (type) => VIDEO_REF_TYPES.includes(type);

const SCENARIO_ALL_DISCOUNT = 'all_discount';
const SCENARIO_GROUP_DISCOUNT = 'group_discount';
const SCENARIO_MODEL_FIXED_PRICE = 'model_fixed_price';
const SCENARIO_MODEL_RATIO = 'model_ratio';
const SCENARIO_VIDEO_REFERENCE = 'video_reference';

// 需要选择具体模型（而非整体/分组）的场景。
const MODEL_SCOPED_SCENARIOS = [
  SCENARIO_MODEL_FIXED_PRICE,
  SCENARIO_MODEL_RATIO,
  SCENARIO_VIDEO_REFERENCE,
];
const isModelScoped = (scenario) => MODEL_SCOPED_SCENARIOS.includes(scenario);

// 参考视频计价方式下拉选项。
const videoRefModeOptions = (t) => [
  { value: TYPE_VIDEO_REF_FACTOR, label: t('参考秒打折/倍率（0=免费,0.5=半价）') },
  { value: TYPE_VIDEO_REF_PRICE, label: t('参考秒固定单价') },
  { value: TYPE_VIDEO_REF_FLAT, label: t('参考整段固定总价') },
  { value: TYPE_VIDEO_REF_CAP, label: t('参考秒数封顶（秒）') },
];

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
    apply_group_ratio: Boolean(rule.apply_group_ratio),
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
          apply_group_ratio: Boolean(rule.apply_group_ratio),
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
  if (type === TYPE_VIDEO_REF_FACTOR) return t('参考秒倍率');
  if (type === TYPE_VIDEO_REF_PRICE) return t('参考秒单价');
  if (type === TYPE_VIDEO_REF_FLAT) return t('参考整段固定价');
  if (type === TYPE_VIDEO_REF_CAP) return t('参考秒封顶');
  return t('整体倍率');
};

// 参考视频规则的数值标签。
const videoRefValueLabel = (type, t) => {
  if (type === TYPE_VIDEO_REF_PRICE) return t('参考每秒单价');
  if (type === TYPE_VIDEO_REF_FLAT) return t('参考整段固定总价');
  if (type === TYPE_VIDEO_REF_CAP) return t('参考秒数封顶（秒）');
  return t('参考秒倍率（0=免费,0.5=半价）');
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
  apply_group_ratio: false,
  disabled: false,
};

const optionKey = (value) => String(value ?? '');

const uniqueBy = (items, getKey) => {
  const seen = new Set();
  const result = [];
  for (const item of items) {
    const key = getKey(item);
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(item);
  }
  return result;
};

const priceLabel = (info, t) => {
  if (!info || !info.exists) return t('未配置价格');
  return info.use_price ? `${t('固定单价')} ${info.price}` : `${t('按量倍率')} ${info.ratio}`;
};

const valuesDiffer = (values) => new Set(values.map((value) => String(value))).size > 1;

const formatNumber = (value) => {
  const num = Number(value);
  if (!Number.isFinite(num)) return '-';
  return Number(num.toFixed(6)).toString();
};

const getContextGroupRatio = (context, groupName) => {
  if (!context) return undefined;
  if (!groupName) return context.user?.current_group_ratio;
  return (context.groups || []).find((group) => group.name === groupName)?.ratio;
};

const buildEffectivePrice = (modelInfo, groupRatio, t) => {
  if (!modelInfo || !modelInfo.exists) {
    return {
      kind: t('未配置价格'),
      base: '-',
      groupRatio: formatNumber(groupRatio),
      final: '-',
    };
  }
  const base = modelInfo.use_price ? modelInfo.price : modelInfo.ratio;
  const final = Number.isFinite(Number(base)) && Number.isFinite(Number(groupRatio))
    ? Number(base) * Number(groupRatio)
    : undefined;
  return {
    kind: modelInfo.use_price ? t('固定单价') : t('按量倍率'),
    base: formatNumber(base),
    groupRatio: formatNumber(groupRatio),
    final: formatNumber(final),
  };
};

const buildOverrideModelInfo = (originalInfo, rule) => {
  if (rule.type === TYPE_MODEL_PRICE) {
    return { exists: true, use_price: true, price: Number(rule.value) || 0 };
  }
  if (rule.type === TYPE_MODEL_RATIO) {
    return { exists: true, use_price: false, ratio: Number(rule.value) || 0 };
  }
  return originalInfo;
};

const inferScenario = (rule) => {
  if (rule.type === TYPE_MODEL_PRICE) return SCENARIO_MODEL_FIXED_PRICE;
  if (rule.type === TYPE_MODEL_RATIO) return SCENARIO_MODEL_RATIO;
  if (isVideoRefType(rule.type)) return SCENARIO_VIDEO_REFERENCE;
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
  if (scenario === SCENARIO_VIDEO_REFERENCE) {
    // 默认参考秒倍率，进入后可在「参考计价方式」下拉里切换。
    return { ...rule, type: isVideoRefType(rule.type) ? rule.type : TYPE_VIDEO_REF_FACTOR };
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
  { value: SCENARIO_VIDEO_REFERENCE, label: t('给用户视频参考秒单独定价') },
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
  if (rule.type === TYPE_VIDEO_REF_FACTOR) {
    return t('用户价格规则预览：{{user}} 使用 {{scope}} 时，参考视频秒数按 ×{{value}} 计价（0=免费,0.5=半价）。', {
      user,
      scope,
      value: rule.value || 0,
    });
  }
  if (rule.type === TYPE_VIDEO_REF_PRICE) {
    return t('用户价格规则预览：{{user}} 使用 {{scope}} 时，参考视频秒数按每秒 {{value}} 计价（与生成秒脱钩）。', {
      user,
      scope,
      value: rule.value || 0,
    });
  }
  if (rule.type === TYPE_VIDEO_REF_FLAT) {
    return t('用户价格规则预览：{{user}} 使用 {{scope}} 时，参考视频部分固定收 {{value}}（不论参考多少秒）。', {
      user,
      scope,
      value: rule.value || 0,
    });
  }
  if (rule.type === TYPE_VIDEO_REF_CAP) {
    return t('用户价格规则预览：{{user}} 使用 {{scope}} 时，参考视频秒数最多按 {{value}} 秒计（超出不收）。', {
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
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [selectedModels, setSelectedModels] = useState([]);
  const [userContexts, setUserContexts] = useState([]);
  const [contextLoading, setContextLoading] = useState(false);

  useEffect(() => {
    const parsed = parseConfig(options.UserPricingOverride);
    setRules(normalizeRules(parsed.rules));
  }, [options.UserPricingOverride]);

  const rawValue = useMemo(() => buildRawValue(rules), [rules]);
  const selectedUserOptions = useMemo(
    () => uniqueBy(
      [
        ...userOptions,
        ...userContexts.map((context) => ({
          id: context.user?.id,
          username: context.user?.username,
          group: context.user?.group,
        })),
      ].filter((user) => user.id),
      (user) => user.id,
    ),
    [userOptions, userContexts],
  );
  const groupOptions = useMemo(
    () => uniqueBy(
      userContexts.flatMap((context) => context.groups || []),
      (group) => group.name,
    ).sort((a, b) => a.name.localeCompare(b.name)),
    [userContexts],
  );
  const modelOptions = useMemo(() => {
    const models = ruleForm.group_pattern
      ? userContexts.flatMap((context) =>
        (context.groups || []).find((group) => group.name === ruleForm.group_pattern)?.models || [],
      )
      : userContexts.flatMap((context) => context.models || []);
    return [...new Set(models)].sort((a, b) => a.localeCompare(b));
  }, [ruleForm.group_pattern, userContexts]);
  const selectedGroupRatios = useMemo(() => {
    if (userContexts.length === 0) return [];
    if (!ruleForm.group_pattern) {
      return userContexts.map((context) => ({
        user: context.user,
        ratio: context.user?.current_group_ratio,
      }));
    }
    return userContexts.map((context) => ({
      user: context.user,
      ratio: (context.groups || []).find((group) => group.name === ruleForm.group_pattern)?.ratio,
    }));
  }, [ruleForm.group_pattern, userContexts]);
  const selectedModelPriceLabels = useMemo(() => {
    const models =
      selectedModels.length > 0
        ? selectedModels
        : ruleForm.model_pattern
          ? [ruleForm.model_pattern]
          : [];
    return models.map((modelName) => {
      const labels = userContexts.map((context) => priceLabel(context.model_prices?.[modelName], t));
      return {
        modelName,
        labels: [...new Set(labels)],
      };
    });
  }, [ruleForm.model_pattern, selectedModels, t, userContexts]);
  const batchPreview = useMemo(() => {
    const userCount = selectedUsers.length || (ruleForm.user_id ? 1 : 0);
    const modelCount =
      isModelScoped(scenario)
        ? selectedModels.length || (ruleForm.model_pattern ? 1 : 0)
        : 1;
    const ruleCount = userCount * modelCount;
    if (ruleCount <= 1) {
      return ruleSummary(ruleForm, t);
    }
    return t('批量预览：将生成 {{count}} 条用户价格规则。', { count: ruleCount });
  }, [ruleForm, scenario, selectedModels.length, selectedUsers.length, t]);
  const priceComparisonRows = useMemo(() => {
    if (userContexts.length === 0) return [];
    const models =
      isModelScoped(scenario)
        ? selectedModels.length > 0
          ? selectedModels
          : ruleForm.model_pattern
            ? [ruleForm.model_pattern]
            : []
        : [''];
    return userContexts.flatMap((context) => {
      const groupRatio = getContextGroupRatio(context, ruleForm.group_pattern);
      return models.map((modelName) => {
        const originalInfo = modelName ? context.model_prices?.[modelName] : null;
        const original = modelName
          ? buildEffectivePrice(originalInfo, groupRatio, t)
          : {
            kind: t('整体倍率'),
            base: '-',
            groupRatio: formatNumber(groupRatio),
            final: formatNumber(groupRatio),
          };
        const overrideGroupRatio = ruleForm.type === TYPE_RATIO ? Number(ruleForm.value) || 0 : groupRatio;
        const overrideInfo = modelName ? buildOverrideModelInfo(originalInfo, ruleForm) : null;
        const override = modelName
          ? buildEffectivePrice(overrideInfo, overrideGroupRatio, t)
          : {
            kind: t('整体倍率'),
            base: '-',
            groupRatio: formatNumber(overrideGroupRatio),
            final: formatNumber(overrideGroupRatio),
          };
        return {
          key: `${context.user?.id}-${modelName || 'all'}`,
          user: `${context.user?.id} / ${context.user?.username} / ${context.user?.group}`,
          group: ruleForm.group_pattern || t('当前用户分组'),
          model: modelName || t('全部模型'),
          original,
          override,
        };
      });
    });
  }, [ruleForm, scenario, selectedModels, t, userContexts]);
  const hasPriceComparisonDifference = useMemo(
    () => priceComparisonRows.some((row) => row.original.final !== row.override.final || row.original.kind !== row.override.kind),
    [priceComparisonRows],
  );

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

  const loadUserContexts = async (userIds) => {
    const ids = [...new Set((userIds || []).filter(Boolean))];
    if (ids.length === 0) {
      setUserContexts([]);
      return;
    }
    setContextLoading(true);
    try {
      const results = await Promise.all(
        ids.map(async (userId) => {
          const res = await API.get(`/api/user/${userId}/pricing_context`);
          if (!res?.data?.success) {
            throw new Error(res?.data?.message || t('获取用户价格上下文失败'));
          }
          return res.data.data;
        }),
      );
      setUserContexts(results);
      if (results.length === 1) {
        const context = results[0];
        setRuleForm((previous) => ({
          ...previous,
          user_id: context.user?.id || previous.user_id,
          username: context.user?.username || previous.username,
          user_group: context.user?.group || previous.user_group,
        }));
      }
    } catch (error) {
      showError(error.message || t('获取用户价格上下文失败'));
      setUserContexts([]);
    } finally {
      setContextLoading(false);
    }
  };

  const openRuleModal = async (rule = null) => {
    setEditingRuleId(rule?.id || '');
    setRuleForm(rule || emptyRule);
    setScenario(inferScenario(rule || emptyRule));
    setSelectedUsers(rule?.user_id ? [rule.user_id] : []);
    setSelectedModels(rule?.model_pattern ? [rule.model_pattern] : []);
    setUserContexts([]);
    if (rule?.user_id) {
      await loadUserContexts([rule.user_id]);
    } else {
      await searchUsers();
    }
    setModalVisible(true);
  };

  const saveRule = () => {
    if (selectedUsers.length === 0 && !Number(ruleForm.user_id)) {
      showError(t('请选择用户'));
      return;
    }
    if (
      (isModelScoped(scenario)) &&
      selectedModels.length === 0 &&
      !String(ruleForm.model_pattern || '').trim()
    ) {
      showError(t('请选择模型或填写模型通配符'));
      return;
    }
    const targetUsers = userContexts.length > 0
      ? userContexts.map((context) => context.user)
      : [{
        id: Number(ruleForm.user_id),
        username: String(ruleForm.username || '').trim(),
        group: String(ruleForm.user_group || '').trim(),
      }];
    const targetModels =
      isModelScoped(scenario)
        ? selectedModels.length > 0
          ? selectedModels
          : [String(ruleForm.model_pattern || '').trim()]
        : [''];
    const nextRules = [];
    for (const user of targetUsers) {
      for (const modelPattern of targetModels) {
        nextRules.push({
          ...ruleForm,
          id: editingRuleId && targetUsers.length === 1 && targetModels.length === 1 ? editingRuleId : `rule-${Date.now()}-${user.id}-${modelPattern || 'all'}`,
          user_id: Number(user.id),
          username: String(user.username || '').trim(),
          user_group: String(user.group || '').trim(),
          group_pattern: String(ruleForm.group_pattern || '').trim(),
          model_pattern: String(modelPattern || '').trim(),
          value: Number(ruleForm.value) || 0,
        });
      }
    }
    setRules((previous) =>
      editingRuleId
        ? [...previous.filter((item) => item.id !== editingRuleId), ...nextRules]
        : [...previous, ...nextRules],
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
        width={1200}
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
              multiple
              showClear
              maxTagCount={3}
              showRestTagsPopover
              value={selectedUsers}
              placeholder={t('搜索并选择用户')}
              style={{ width: '100%', marginBottom: 12 }}
              onSearch={searchUsers}
              onChange={(value) => {
                const ids = Array.isArray(value) ? value : [];
                const selected = selectedUserOptions.find((user) => user.id === ids[0]);
                setSelectedUsers(ids);
                setRuleForm({
                  ...ruleForm,
                  user_id: selected?.id || 0,
                  username: selected?.username || '',
                  user_group: selected?.group || '',
                  group_pattern: '',
                  model_pattern: '',
                });
                if (ids.length > 0) {
                  loadUserContexts(ids);
                } else {
                  setUserContexts([]);
                }
              }}
            >
              {selectedUserOptions.map((user) => (
                <Select.Option key={user.id} value={user.id}>
                  {user.id} / {user.username} / {user.group}
                </Select.Option>
              ))}
            </Select>
            {userContexts.length > 0 ? (
              <div className='mb-3 text-sm text-gray-500'>
                {t('已选择')}：{userContexts.map((context) => `${context.user.id} / ${context.user.username} / ${context.user.group}`).join('，')}
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
                  onChange={(value) => {
                    setSelectedModels([]);
                    setRuleForm({ ...ruleForm, group_pattern: value || '', model_pattern: '' });
                  }}
                >
                  {groupOptions.map((group) => (
                    <Select.Option key={group.name} value={group.name}>
                      {group.name}{group.desc ? ` / ${group.desc}` : ''} / {group.models?.length || 0} {t('个模型')}
                    </Select.Option>
                  ))}
                </Select>
              </>
            ) : null}
            {isModelScoped(scenario) ? (
              <>
                <div className='mb-2 font-medium text-gray-700'>{t('适用模型')}</div>
                <Select
                  filter
                  multiple
                  showClear
                  maxTagCount={3}
                  showRestTagsPopover
                  value={selectedModels}
                  placeholder={t('全部模型')}
                  style={{ width: '100%', marginBottom: 16 }}
                  onChange={(value) => {
                    const models = Array.isArray(value) ? value : [];
                    setSelectedModels(models);
                    setRuleForm({ ...ruleForm, model_pattern: models[0] || '' });
                  }}
                >
                  {modelOptions.map((modelName) => (
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
                  onChange={(value) => {
                    setSelectedModels(value ? [value] : []);
                    setRuleForm({ ...ruleForm, model_pattern: value });
                  }}
                />
              </>
            ) : null}
            {scenario === SCENARIO_VIDEO_REFERENCE ? (
              <>
                <div className='mb-2 font-medium text-gray-700'>{t('参考计价方式')}</div>
                <Select
                  value={isVideoRefType(ruleForm.type) ? ruleForm.type : TYPE_VIDEO_REF_FACTOR}
                  style={{ width: '100%', marginBottom: 16 }}
                  onChange={(value) => setRuleForm({ ...ruleForm, type: value })}
                >
                  {videoRefModeOptions(t).map((item) => (
                    <Select.Option key={item.value} value={item.value}>
                      {item.label}
                    </Select.Option>
                  ))}
                </Select>
              </>
            ) : null}
            <div className='mb-2 font-medium text-gray-700'>
              {ruleForm.type === TYPE_MODEL_PRICE
                ? t('固定单价')
                : isVideoRefType(ruleForm.type)
                  ? videoRefValueLabel(ruleForm.type, t)
                  : t('倍率数值')}
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
                : ruleForm.type === TYPE_VIDEO_REF_FACTOR
                  ? t('参考秒倍率：0=参考秒免费，0.5=半价，1=与生成秒同价；只影响参考视频那部分秒数。')
                  : ruleForm.type === TYPE_VIDEO_REF_PRICE
                    ? t('参考每秒单价：参考视频按此单价计，与生成秒的价格脱钩。')
                    : ruleForm.type === TYPE_VIDEO_REF_FLAT
                      ? t('参考整段固定总价：不论参考多少秒，参考部分固定收这么多。')
                      : ruleForm.type === TYPE_VIDEO_REF_CAP
                        ? t('参考秒数封顶（秒）：参考最多按这么多秒计，超出不收。')
                        : t('倍率填写 0.8 表示八折，1 表示原价，0 表示免费。')}
            </div>
            {ruleForm.type === TYPE_VIDEO_REF_PRICE || ruleForm.type === TYPE_VIDEO_REF_FLAT ? (
              <div className='mt-4 flex items-center justify-between'>
                <div>
                  <div className='font-medium text-gray-700'>{t('固定价同时享受分组折扣')}</div>
                  <div className='text-sm text-gray-500'>
                    {t('默认关闭：你填多少就收多少；开启后参考固定价会再乘以用户的分组倍率。')}
                  </div>
                </div>
                <Switch
                  checked={Boolean(ruleForm.apply_group_ratio)}
                  onChange={(checked) => setRuleForm({ ...ruleForm, apply_group_ratio: checked })}
                />
              </div>
            ) : null}
            {selectedGroupRatios.length > 1 && valuesDiffer(selectedGroupRatios.map((item) => item.ratio)) ? (
              <Card className='mt-4' bodyStyle={{ padding: 12 }}>
                <div className='mb-2 font-medium text-red-600'>{t('分组倍率不一致')}</div>
                <div className='text-sm text-gray-600'>
                  {selectedGroupRatios.map((item) => (
                    <div key={item.user?.id}>
                      {item.user?.id} / {item.user?.username} / {item.user?.group}：{item.ratio ?? '-'}
                    </div>
                  ))}
                </div>
              </Card>
            ) : null}
            {selectedModelPriceLabels.length > 1 && valuesDiffer(selectedModelPriceLabels.flatMap((item) => item.labels)) ? (
              <Card className='mt-4' bodyStyle={{ padding: 12 }}>
                <div className='mb-2 font-medium text-red-600'>{t('模型原始价格不一致')}</div>
                <div className='text-sm text-gray-600'>
                  {selectedModelPriceLabels.map((item) => (
                    <div key={item.modelName}>
                      {item.modelName}：{item.labels.join(' / ')}
                    </div>
                  ))}
                </div>
              </Card>
            ) : null}
            {priceComparisonRows.length > 0 ? (
              <Card className='mt-4' bodyStyle={{ padding: 12 }}>
                <div className={`mb-2 font-medium ${hasPriceComparisonDifference ? 'text-red-600' : 'text-gray-700'}`}>
                  {t('覆盖前后价格对比')}
                </div>
                <Table
                  size='small'
                  pagination={false}
                  rowKey='key'
                  dataSource={priceComparisonRows}
                  scroll={{ x: 1120 }}
                  columns={[
                    {
                      title: t('用户'),
                      dataIndex: 'user',
                      width: 260,
                    },
                    {
                      title: t('分组'),
                      dataIndex: 'group',
                      width: 160,
                    },
                    {
                      title: t('模型'),
                      dataIndex: 'model',
                      width: 260,
                    },
                    {
                      title: t('原计费'),
                      width: 220,
                      render: (_, row) => `${row.original.kind} ${row.original.base} × ${row.original.groupRatio} = ${row.original.final}`,
                    },
                    {
                      title: t('覆盖后'),
                      width: 220,
                      render: (_, row) => `${row.override.kind} ${row.override.base} × ${row.override.groupRatio} = ${row.override.final}`,
                    },
                  ]}
                />
              </Card>
            ) : null}
            <Card className='mt-4' bodyStyle={{ padding: 12 }}>
              {batchPreview}
            </Card>
          </Form>
        </Spin>
      </Modal>
    </Space>
  );
}
