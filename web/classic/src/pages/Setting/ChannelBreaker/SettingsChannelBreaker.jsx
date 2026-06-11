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

import React, { useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import {
  API,
  compareObjects,
  parseHttpStatusCodeRules,
  showError,
  showSuccess,
  showWarning,
  toBoolean,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';
import HttpStatusCodeRulesInput from '../../../components/settings/HttpStatusCodeRulesInput';

const OPTION_KEYS = [
  'ChannelBreakerEnabled',
  'ChannelBreakerFailureLimit',
  'ChannelBreakerCooldownSeconds',
  'ChannelBreakerProbeCount',
  'ChannelBreakerProbeSuccessCount',
  'ChannelBreakerExcludePaths',
  'ChannelBreakerRules',
  'monitor_setting.bark_alert_enabled',
  'monitor_setting.bark_alert_url',
  'monitor_setting.low_balance_alert_enabled',
  'monitor_setting.low_balance_threshold_cny',
  'monitor_setting.channel_breaker_alert_enabled',
  'AutomaticDisableKeywords',
  'ChannelBreakerFailureStatusCodes',
];

const defaultInputs = {
  ChannelBreakerEnabled: false,
  ChannelBreakerFailureLimit: '5',
  ChannelBreakerCooldownSeconds: '60',
  ChannelBreakerProbeCount: '5',
  ChannelBreakerProbeSuccessCount: '3',
  ChannelBreakerExcludePaths: '/v1/videos',
  ChannelBreakerRules: '[]',
  'monitor_setting.bark_alert_enabled': true,
  'monitor_setting.bark_alert_url':
    'https://bark.aigod.one/kFRNZMUXcuQ6c4ccrUgQ3W/',
  'monitor_setting.low_balance_alert_enabled': true,
  'monitor_setting.low_balance_threshold_cny': 10,
  'monitor_setting.channel_breaker_alert_enabled': true,
  AutomaticDisableKeywords: '',
  ChannelBreakerFailureStatusCodes:
    '100-199,300-399,401-407,409-499,500-503,505-523,525-599',
};

const scopeOptions = [
  { label: '全局', value: 'global' },
  { label: '分组', value: 'group' },
  { label: '模型', value: 'model' },
  { label: '渠道', value: 'channel' },
];

const ruleTemplates = [
  {
    name: '稳健默认',
    failure_limit: 5,
    cooldown_seconds: 60,
    probe_count: 5,
    probe_success_count: 3,
    failure_status_codes: '429,500-599',
    failure_keywords:
      'rate limit\ntemporarily unavailable\noverloaded\nserver error',
    exclude_paths: '/v1/videos',
  },
  {
    name: '严格保护',
    failure_limit: 3,
    cooldown_seconds: 120,
    probe_count: 5,
    probe_success_count: 4,
    failure_status_codes: '401,403,429,500-599',
    failure_keywords:
      'insufficient quota\npermission denied\nrate limit\noverloaded',
    exclude_paths: '/v1/videos',
  },
  {
    name: '宽松容忍',
    failure_limit: 10,
    cooldown_seconds: 30,
    probe_count: 5,
    probe_success_count: 2,
    failure_status_codes: '500-599',
    failure_keywords: 'temporarily unavailable\noverloaded\nserver error',
    exclude_paths: '/v1/videos',
  },
  {
    name: '视频/异步任务排除',
    failure_limit: 20,
    cooldown_seconds: 30,
    probe_count: 5,
    probe_success_count: 2,
    failure_status_codes: '500-599',
    failure_keywords: '',
    exclude_paths: '/v1/videos',
    disable_breaker: true,
  },
  {
    name: '上游余额不足立即禁用',
    failure_limit: 5,
    cooldown_seconds: 60,
    probe_count: 5,
    probe_success_count: 3,
    failure_status_codes: '429,500-599',
    failure_keywords: '',
    exclude_paths: '/v1/videos',
    instant_disable_enabled: true,
    instant_disable_status_codes: '403',
    instant_disable_keywords:
      'insufficient account balance\ninsufficient_user_quota\n预扣费额度失败\nInsufficient account balance',
  },
];

// 内置规则：与后端 defaultChannelBreakerRuntimeRule 的立即禁用默认值保持一致。
// 开箱即用、独立于全局熔断开关，仅用于展示，不可编辑/删除。
const BUILTIN_INSTANT_DISABLE_RULE = {
  id: '__builtin_instant_disable__',
  builtin: true,
  name: '上游余额不足立即禁用',
  enabled: true,
  scope: 'global',
  targets: [],
  instant_disable_enabled: true,
  instant_disable_status_codes: '403',
  instant_disable_keywords:
    'insufficient account balance\ninsufficient user quota\ninsufficient_user_quota\n预扣费额度失败',
};

export default function SettingsChannelBreaker(props) {
  const { t } = useTranslation();
  const { Text, Title } = Typography;
  const [loading, setLoading] = useState(false);
  const [statusLoading, setStatusLoading] = useState(false);
  const [breakerStatuses, setBreakerStatuses] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [breakerHistory, setBreakerHistory] = useState([]);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyTotal, setHistoryTotal] = useState(0);
  const HISTORY_PAGE_SIZE = 10;
  const [rules, setRules] = useState([]);
  const [ruleModalVisible, setRuleModalVisible] = useState(false);
  const [editingRuleIndex, setEditingRuleIndex] = useState(-1);
  const [editingRule, setEditingRule] = useState(null);
  const [inputs, setInputs] = useState(defaultInputs);
  const [inputsRow, setInputsRow] = useState(defaultInputs);
  const refForm = useRef();

  const parsedChannelBreakerStatusCodes = parseHttpStatusCodeRules(
    inputs.ChannelBreakerFailureStatusCodes || '',
  );

  function normalizeValue(key, value) {
    if (key === 'ChannelBreakerFailureStatusCodes') {
      return parsedChannelBreakerStatusCodes.normalized;
    }
    if (typeof value === 'boolean') {
      return String(value);
    }
    return String(value ?? '');
  }

  function parseRules(value) {
    try {
      const parsed = JSON.parse(value || '[]');
      return Array.isArray(parsed) ? parsed : [];
    } catch (error) {
      return [];
    }
  }

  function stringifyRules(nextRules) {
    return JSON.stringify(nextRules, null, 2);
  }

  function syncRules(nextRules) {
    setRules(nextRules);
    setInputs({ ...inputs, ChannelBreakerRules: stringifyRules(nextRules) });
  }

  function normalizeRule(rule) {
    const probeCount = Number(rule.probe_count || 5);
    const probeSuccessCount = Math.min(
      Number(rule.probe_success_count || 3),
      probeCount,
    );
    return {
      id:
        rule.id ||
        `rule-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      name: rule.name || '自定义规则',
      enabled: rule.enabled !== false,
      scope: rule.scope || 'group',
      targets: Array.isArray(rule.targets)
        ? rule.targets
        : String(rule.targets || '')
            .split(/[\n,，]/)
            .map((item) => item.trim())
            .filter(Boolean),
      failure_limit: Number(rule.failure_limit || 5),
      cooldown_seconds: Number(rule.cooldown_seconds || 60),
      probe_count: probeCount,
      probe_success_count: probeSuccessCount,
      failure_status_codes: rule.failure_status_codes || '429,500-599',
      failure_keywords: rule.failure_keywords || '',
      exclude_paths: rule.exclude_paths || '',
      disable_breaker: !!rule.disable_breaker,
      only_key_breaker: !!rule.only_key_breaker,
      ignore_client_error_4xx: !!rule.ignore_client_error_4xx,
      instant_disable_enabled: !!rule.instant_disable_enabled,
      instant_disable_status_codes: rule.instant_disable_status_codes || '',
      instant_disable_keywords: rule.instant_disable_keywords || '',
    };
  }

  function openRuleModal(rule, index) {
    setEditingRuleIndex(index);
    setEditingRule(
      normalizeRule(
        rule || {
          ...ruleTemplates[0],
          scope: 'group',
          targets: [],
        },
      ),
    );
    setRuleModalVisible(true);
  }

  function addRuleFromTemplate(template) {
    const nextRule = normalizeRule({
      ...template,
      id: '',
      scope: template.disable_breaker ? 'model' : 'group',
      targets: [],
    });
    syncRules([...rules, nextRule]);
  }

  function saveEditingRule() {
    const nextRule = normalizeRule(editingRule || {});
    if (!nextRule.name.trim()) {
      showError(t('规则名称不能为空'));
      return;
    }
    if (nextRule.scope !== 'global' && nextRule.targets.length === 0) {
      showError(t('非全局规则必须填写匹配对象'));
      return;
    }
    const nextRules = [...rules];
    if (editingRuleIndex >= 0) {
      nextRules[editingRuleIndex] = nextRule;
    } else {
      nextRules.push(nextRule);
    }
    syncRules(nextRules);
    setRuleModalVisible(false);
  }

  function deleteRule(index) {
    syncRules(rules.filter((_, i) => i !== index));
  }

  function toggleRule(index, enabled) {
    const nextRules = [...rules];
    nextRules[index] = { ...nextRules[index], enabled };
    syncRules(nextRules);
  }

  async function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    if (!parsedChannelBreakerStatusCodes.ok) {
      const details =
        parsedChannelBreakerStatusCodes.invalidTokens?.length > 0
          ? `: ${parsedChannelBreakerStatusCodes.invalidTokens.join(', ')}`
          : '';
      return showError(`${t('熔断失败状态码格式不正确')}${details}`);
    }

    const requestQueue = updateArray.map((item) =>
      API.put('/api/option/', {
        key: item.key,
        value: normalizeValue(item.key, inputs[item.key]),
      }),
    );

    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (res.includes(undefined)) {
          return showError(
            requestQueue.length > 1 ? t('部分保存失败，请重试') : t('保存失败'),
          );
        }
        for (let i = 0; i < res.length; i++) {
          if (!res[i].data.success) {
            return showError(res[i].data.message);
          }
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => showError(t('保存失败，请重试')))
      .finally(() => setLoading(false));
  }

  async function saveRulesOnly() {
    setLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'ChannelBreakerRules',
        value: normalizeValue('ChannelBreakerRules', inputs.ChannelBreakerRules),
      });
      if (res.data?.success) {
        showSuccess(t('熔断规则已保存'));
        props.refresh();
      } else {
        showError(res.data?.message || t('保存失败'));
      }
    } catch (e) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  async function fetchBreakerStatuses() {
    try {
      setStatusLoading(true);
      const res = await API.get('/api/option/channel_breaker/statuses');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setBreakerStatuses(data?.items || []);
    } catch (error) {
      showError(t('获取熔断状态失败'));
    } finally {
      setStatusLoading(false);
    }
  }

  async function fetchBreakerHistory(page = historyPage) {
    try {
      setHistoryLoading(true);
      const res = await API.get(
        `/api/option/channel_breaker/logs?page=${page}&page_size=${HISTORY_PAGE_SIZE}`,
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setBreakerHistory(data?.items || []);
      setHistoryTotal(data?.total || 0);
      setHistoryPage(page);
    } catch (error) {
      showError(t('获取熔断历史失败'));
    } finally {
      setHistoryLoading(false);
    }
  }

  async function clearBreakerStatus(record) {
    try {
      setStatusLoading(true);
      const res = await API.post('/api/option/channel_breaker/clear', {
        state_key: record.state_key,
      });
      const { success, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      showSuccess(t('已解除熔断'));
      await fetchBreakerStatuses();
    } catch (error) {
      showError(t('解除熔断失败'));
    } finally {
      setStatusLoading(false);
    }
  }

  function formatTime(value) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '-';
    return date.toLocaleString();
  }

  function renderStateTag(state) {
    if (state === 'open') {
      return (
        <Tag color='red' shape='circle'>
          {t('熔断中')}
        </Tag>
      );
    }
    if (state === 'half-open') {
      return (
        <Tag color='orange' shape='circle'>
          {t('探测中')}
        </Tag>
      );
    }
    return (
      <Tag color='green' shape='circle'>
        {t('正常')}
      </Tag>
    );
  }

  function renderRuleField(label, control, extraText) {
    return (
      <div style={{ marginBottom: 12 }}>
        <Text strong style={{ display: 'block', marginBottom: 6 }}>
          {label}
        </Text>
        {control}
        {extraText && (
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {extraText}
          </Text>
        )}
      </div>
    );
  }

  const breakerStatusColumns = [
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      render: (text, record) => (
        <div>
          <Text strong>{text || `#${record.channel_id}`}</Text>
          <br />
          <Text type='tertiary' size='small'>
            #{record.channel_id}
            {record.channel_group ? ` / ${record.channel_group}` : ''}
          </Text>
        </div>
      ),
    },
    {
      title: t('对象'),
      dataIndex: 'key_hash',
      render: (text, record) => {
        if (!text) return <Tag>{t('整个渠道')}</Tag>;
        const keyLabel =
          typeof record.key_index === 'number'
            ? `${t('Key')} #${record.key_index + 1}`
            : t('Key');
        return (
          <div>
            <Tag color='blue'>{keyLabel}</Tag>
            <br />
            <Text type='tertiary' size='small'>
              {text}
            </Text>
          </div>
        );
      },
    },
    {
      title: t('状态'),
      dataIndex: 'state',
      render: renderStateTag,
    },
    {
      title: t('探测进度'),
      dataIndex: 'probe_total',
      render: (_, record) => (
        <Text>
          {record.probe_success || 0}/{inputs.ChannelBreakerProbeSuccessCount}{' '}
          {t('成功')} · {record.probe_total || 0}/
          {inputs.ChannelBreakerProbeCount} {t('完成')}
        </Text>
      ),
    },
    {
      title: t('冷却剩余'),
      dataIndex: 'cooldown_remaining_seconds',
      render: (value, record) => {
        if (record.state === 'half-open') return t('正在探测');
        if (!value || value <= 0) return t('可探测');
        return `${value}${t('秒')}`;
      },
    },
    {
      title: t('开始时间'),
      dataIndex: 'opened_at',
      render: formatTime,
    },
    {
      title: t('权重/优先级'),
      dataIndex: 'weight',
      render: (_, record) => (
        <Text>
          {record.weight || 0} / {record.priority || 0}
        </Text>
      ),
    },
    {
      title: t('命中规则'),
      dataIndex: 'rule_name',
      render: (text, record) => (
        <div>
          <Text>{text || '-'}</Text>
          {(record.model || record.group) && (
            <>
              <br />
              <Text type='tertiary' size='small'>
                {[record.group, record.model].filter(Boolean).join(' / ')}
              </Text>
            </>
          )}
        </div>
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      render: (_, record) => (
        <Popconfirm
          title={t('确认解除该熔断？')}
          content={t('解除后该渠道或密钥会立即重新参与正常调度。')}
          onConfirm={() => clearBreakerStatus(record)}
        >
          <Button size='small' type='warning'>
            {t('解除熔断')}
          </Button>
        </Popconfirm>
      ),
    },
  ];

  const ruleColumns = [
    {
      title: t('规则名称'),
      dataIndex: 'name',
      render: (text, record) => (
        <div>
          <Text strong>{t(text)}</Text>
          <br />
          {record.builtin ? (
            <Tag color='blue' size='small'>
              {t('内置')}
            </Tag>
          ) : (
            <Tag color={record.enabled ? 'green' : 'grey'} size='small'>
              {record.enabled ? t('启用') : t('停用')}
            </Tag>
          )}
          {record.disable_breaker && (
            <Tag color='orange' size='small'>
              {t('不参与熔断')}
            </Tag>
          )}
          {record.instant_disable_enabled && (
            <Tag color='red' size='small'>
              {t('立即禁用')}
            </Tag>
          )}
        </div>
      ),
    },
    {
      title: t('作用范围'),
      dataIndex: 'scope',
      render: (scope) =>
        scopeOptions.find((option) => option.value === scope)?.label || scope,
    },
    {
      title: t('匹配对象'),
      dataIndex: 'targets',
      render: (targets, record) => {
        if (record.scope === 'global') return t('全部请求');
        if (!targets || targets.length === 0) return '-';
        return (
          <Space wrap>
            {targets.map((target) => (
              <Tag key={target}>{target}</Tag>
            ))}
          </Space>
        );
      },
    },
    {
      title: t('失败/冷却'),
      dataIndex: 'failure_limit',
      render: (_, record) =>
        record.builtin
          ? '-'
          : `${record.failure_limit}${t('次')} / ${record.cooldown_seconds}${t('秒')}`,
    },
    {
      title: t('探测要求'),
      dataIndex: 'probe_success_count',
      render: (_, record) =>
        record.builtin
          ? '-'
          : `${record.probe_success_count}/${record.probe_count} ${t('成功')}`,
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      render: (_, record) => {
        if (record.builtin) {
          return <Text type='tertiary'>{t('内置规则，不可编辑')}</Text>;
        }
        const index = rules.findIndex((r) => r.id === record.id);
        return (
          <Space>
            <Button size='small' onClick={() => openRuleModal(record, index)}>
              {t('编辑')}
            </Button>
            <Button
              size='small'
              onClick={() => toggleRule(index, !record.enabled)}
            >
              {record.enabled ? t('停用') : t('启用')}
            </Button>
            <Popconfirm
              title={t('确认删除该规则？')}
              onConfirm={() => deleteRule(index)}
            >
              <Button size='small' type='danger'>
                {t('删除')}
              </Button>
            </Popconfirm>
          </Space>
        );
      },
    },
  ];

  const isInstantDisableLog = (record) =>
    typeof record?.reason === 'string' &&
    record.reason.startsWith('命中立即禁用规则');

  const breakerHistoryColumns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (value) => formatTime(value ? value * 1000 : 0),
    },
    {
      title: t('类型'),
      dataIndex: 'breaker_type',
      render: (_, record) =>
        isInstantDisableLog(record) ? (
          <Tag color='red' size='small'>
            {t('立即禁用')}
          </Tag>
        ) : (
          <Tag color='blue' size='small'>
            {t('熔断')}
          </Tag>
        ),
    },
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      render: (value, record) =>
        value && value !== '' ? value : `#${record.channel_id}`,
    },
    {
      title: t('模型'),
      dataIndex: 'model_name',
      render: (value) => (value && value !== '' ? value : '-'),
    },
    {
      title: t('分组'),
      dataIndex: 'using_group',
      render: (value) => (value && value !== '' ? value : '-'),
    },
    {
      title: t('命中规则'),
      dataIndex: 'rule_name',
      render: (value) => (value && value !== '' ? value : '-'),
    },
    {
      title: t('失败次数'),
      dataIndex: 'failures',
      render: (value, record) => (isInstantDisableLog(record) ? '-' : value),
    },
    {
      title: t('冷却(秒)'),
      dataIndex: 'cooldown_secs',
      render: (value, record) => (isInstantDisableLog(record) ? '-' : value),
    },
  ];

  useEffect(() => {
    const currentInputs = { ...defaultInputs };
    for (const key of OPTION_KEYS) {
      if (!Object.prototype.hasOwnProperty.call(props.options || {}, key)) {
        continue;
      }
      if (
        key === 'ChannelBreakerEnabled' ||
        key === 'monitor_setting.bark_alert_enabled' ||
        key === 'monitor_setting.low_balance_alert_enabled' ||
        key === 'monitor_setting.channel_breaker_alert_enabled'
      ) {
        currentInputs[key] = toBoolean(props.options[key]);
      } else {
        currentInputs[key] = props.options[key] ?? defaultInputs[key];
      }
    }
    setInputs(currentInputs);
    setRules(parseRules(currentInputs.ChannelBreakerRules));
    setInputsRow(structuredClone(currentInputs));
    if (refForm.current) {
      refForm.current.setValues(currentInputs);
    }
  }, [props.options]);

  useEffect(() => {
    fetchBreakerStatuses();
    fetchBreakerHistory(1);
  }, []);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('分组容灾')}>
          <div
            style={{
              border: '1px solid var(--semi-color-border)',
              borderRadius: 8,
              padding: 16,
              marginBottom: 16,
              background: 'var(--semi-color-fill-0)',
            }}
          >
            <Title heading={5} style={{ marginTop: 0 }}>
              {t('它解决什么问题')}
            </Title>
            <Text>
              {t(
                '分组容灾不会禁用你手动启用的渠道，也不会改渠道权重。它只是在上游连续异常时，把异常渠道或异常 key 临时从调度里摘出去，避免用户请求继续打到明显失败的上游。',
              )}
            </Text>
            <Row gutter={16} style={{ marginTop: 16 }}>
              <Col xs={24} sm={8}>
                <Text strong>{t('触发')}</Text>
                <br />
                <Text type='tertiary'>
                  {t(
                    '命中失败状态码、失败关键词或 channel error 后累计失败次数。',
                  )}
                </Text>
              </Col>
              <Col xs={24} sm={8}>
                <Text strong>{t('恢复')}</Text>
                <br />
                <Text type='tertiary'>
                  {t('冷却结束后放真实请求探测，达到成功要求就恢复正常调度。')}
                </Text>
              </Col>
              <Col xs={24} sm={8}>
                <Text strong>{t('人工干预')}</Text>
                <br />
                <Text type='tertiary'>
                  {t(
                    '运营可以在下方表格直接解除熔断，让渠道或 key 立刻恢复调度。',
                  )}
                </Text>
              </Col>
            </Row>
          </div>

          <Banner
            fullMode={false}
            type='info'
            description={t(
              '熔断调度只会临时跳过异常渠道或密钥，不会修改渠道启用状态；冷却后通过真实请求探测恢复。',
            )}
          />

          <Form.Section text={t('当前熔断状态')}>
            <Space style={{ marginBottom: 12 }}>
              <Button onClick={fetchBreakerStatuses} loading={statusLoading}>
                {t('刷新状态')}
              </Button>
              <Text type='tertiary'>
                {t('表格只展示正在熔断或探测中的渠道/key，不展示真实密钥。')}
              </Text>
            </Space>
            <Table
              size='small'
              loading={statusLoading}
              columns={breakerStatusColumns}
              dataSource={breakerStatuses}
              rowKey='state_key'
              pagination={false}
              empty={t('当前没有熔断中的渠道或 key')}
            />
          </Form.Section>

          <Form.Section text={t('历史熔断日志')}>
            <Space style={{ marginBottom: 12 }}>
              <Button
                onClick={() => fetchBreakerHistory(historyPage)}
                loading={historyLoading}
              >
                {t('刷新历史')}
              </Button>
              <Text type='tertiary'>
                {t('记录每一次熔断器打开的历史，便于排查异常渠道。')}
              </Text>
            </Space>
            <Table
              size='small'
              loading={historyLoading}
              columns={breakerHistoryColumns}
              dataSource={breakerHistory}
              rowKey='id'
              pagination={{
                currentPage: historyPage,
                pageSize: HISTORY_PAGE_SIZE,
                total: historyTotal,
                onPageChange: (page) => fetchBreakerHistory(page),
              }}
              empty={t('暂无熔断历史')}
            />
          </Form.Section>

          <Form.Section text={t('熔断规则')}>
            <Banner
              fullMode={false}
              type='warning'
              description={t(
                '规则按优先级匹配：渠道 > 模型 > 分组 > 全局。命中具体规则时，会使用该规则里的失败次数、冷却时间、探测要求、失败状态码、失败关键词和排除路径；没有命中规则时才使用下方全局熔断参数。列表中的「内置」规则为上游余额不足（403 + 余额相关关键词）立即禁用整个渠道的开箱即用保护，独立于全局熔断开关、命中一次即生效，不可编辑或删除。',
              )}
            />
            <Space style={{ margin: '12px 0' }} wrap>
              <Button type='primary' onClick={() => openRuleModal(null, -1)}>
                {t('新增规则')}
              </Button>
              {ruleTemplates.map((template) => (
                <Button
                  key={template.name}
                  onClick={() => addRuleFromTemplate(template)}
                >
                  {t(template.name)}
                </Button>
              ))}
              <Button
                theme='solid'
                type='secondary'
                loading={loading}
                onClick={saveRulesOnly}
              >
                {t('保存规则')}
              </Button>
            </Space>
            <Table
              size='small'
              columns={ruleColumns}
              dataSource={[BUILTIN_INSTANT_DISABLE_RULE, ...rules]}
              rowKey='id'
              pagination={false}
              empty={t('当前没有自定义规则，将使用全局熔断参数')}
            />
          </Form.Section>

          <Form.Section text={t('全局熔断参数')}>
            <Banner
              fullMode={false}
              type='info'
              description={t(
                '全局熔断参数是兜底默认值：没有命中任何熔断规则时使用；新增熔断规则时也会用这些推荐默认值预填，运营只需要改特殊分组、模型或渠道。',
              )}
            />
            <Row gutter={16} style={{ marginTop: 12 }}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field='ChannelBreakerEnabled'
                  label={t('启用熔断调度')}
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({ ...inputs, ChannelBreakerEnabled: value })
                  }
                />
                <Text type='tertiary' size='small'>
                  {t('关闭后不会因为上游错误临时跳过渠道。')}
                </Text>
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field='ChannelBreakerFailureLimit'
                  label={t('连续失败次数')}
                  min={1}
                  step={1}
                  extraText={t('达到该次数后打开熔断。')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ChannelBreakerFailureLimit: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field='ChannelBreakerCooldownSeconds'
                  label={t('冷却时间')}
                  min={1}
                  step={1}
                  suffix={t('秒')}
                  extraText={t('冷却结束后才允许真实请求探测。')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ChannelBreakerCooldownSeconds: String(value),
                    })
                  }
                />
              </Col>
            </Row>

            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field='ChannelBreakerProbeCount'
                  label={t('探测请求数')}
                  min={1}
                  step={1}
                  extraText={t('半开状态最多放行的真实请求数量。')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ChannelBreakerProbeCount: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field='ChannelBreakerProbeSuccessCount'
                  label={t('探测成功要求')}
                  min={1}
                  step={1}
                  extraText={t('达到该成功数后关闭熔断，恢复正常调度。')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ChannelBreakerProbeSuccessCount: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.TextArea
                  field='ChannelBreakerExcludePaths'
                  label={t('熔断排除路径')}
                  placeholder={'/v1/videos'}
                  autosize={{ minRows: 3, maxRows: 6 }}
                  extraText={t('一行一个路径前缀，匹配后不会影响熔断状态。')}
                  onChange={(value) =>
                    setInputs({ ...inputs, ChannelBreakerExcludePaths: value })
                  }
                />
              </Col>
            </Row>

            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <HttpStatusCodeRulesInput
                  label={t('熔断失败状态码')}
                  placeholder={t('例如：401, 403, 429, 500-599')}
                  extraText={t(
                    '仅用于熔断失败计数，由熔断功能单独维护，与自动重试、自动禁用状态码互不影响。',
                  )}
                  field='ChannelBreakerFailureStatusCodes'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ChannelBreakerFailureStatusCodes: value,
                    })
                  }
                  parsed={parsedChannelBreakerStatusCodes}
                  invalidText={t('熔断失败状态码格式不正确')}
                />
                <Form.TextArea
                  label={t('失败关键词')}
                  placeholder={t('一行一个，不区分大小写')}
                  extraText={t(
                    '上游错误包含这些关键词时，会被视为渠道失败，用于自动禁用和熔断失败计数。',
                  )}
                  field='AutomaticDisableKeywords'
                  autosize={{ minRows: 6, maxRows: 12 }}
                  onChange={(value) =>
                    setInputs({ ...inputs, AutomaticDisableKeywords: value })
                  }
                />
              </Col>
            </Row>

            <Form.Section text={t('Bark 系统告警')}>
              <Banner
                fullMode={false}
                type='info'
                description={t('通过配置的 Bark API 发送余额和熔断告警')}
              />
              <Row gutter={16} style={{ marginTop: 12 }}>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Switch
                    field='monitor_setting.bark_alert_enabled'
                    label={t('Bark 系统告警')}
                    size='default'
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'monitor_setting.bark_alert_enabled': value,
                      })
                    }
                  />
                </Col>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Switch
                    field='monitor_setting.channel_breaker_alert_enabled'
                    label={t('熔断 Bark 告警')}
                    size='default'
                    checkedText='｜'
                    uncheckedText='〇'
                    extraText={t('渠道发生熔断时发送 Bark 告警')}
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'monitor_setting.channel_breaker_alert_enabled': value,
                      })
                    }
                  />
                </Col>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Switch
                    field='monitor_setting.low_balance_alert_enabled'
                    label={t('低余额 Bark 告警')}
                    size='default'
                    checkedText='｜'
                    uncheckedText='〇'
                    extraText={t('用户余额低于阈值时发送 Bark 告警')}
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'monitor_setting.low_balance_alert_enabled': value,
                      })
                    }
                  />
                </Col>
              </Row>
              <Row gutter={16}>
                <Col xs={24} sm={16}>
                  <Form.Input
                    label={t('Bark API 地址')}
                    placeholder='https://bark.example.com/device-key/'
                    extraText={t(
                      '用于发送系统余额预警和渠道熔断告警的 Bark 地址',
                    )}
                    field='monitor_setting.bark_alert_url'
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'monitor_setting.bark_alert_url': value.trim(),
                      })
                    }
                  />
                </Col>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.InputNumber
                    label={t('低余额阈值')}
                    step={0.01}
                    min={0}
                    suffix={t('元')}
                    extraText={t('用户余额低于该人民币金额时触发 Bark 告警')}
                    placeholder=''
                    field='monitor_setting.low_balance_threshold_cny'
                    onChange={(value) =>
                      setInputs({
                        ...inputs,
                        'monitor_setting.low_balance_threshold_cny':
                          Number(value),
                      })
                    }
                  />
                </Col>
              </Row>
            </Form.Section>

            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存分组容灾设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form.Section>
      </Form>
      <Modal
        title={editingRuleIndex >= 0 ? t('编辑熔断规则') : t('新增熔断规则')}
        visible={ruleModalVisible}
        onOk={saveEditingRule}
        onCancel={() => setRuleModalVisible(false)}
        okText={t('保存')}
        cancelText={t('取消')}
        width={760}
      >
        {editingRule && (
          <div>
            <Banner
              fullMode={false}
              type='info'
              style={{ marginBottom: 12 }}
              description={t(
                '新增规则已预填稳健默认值：连续失败 5 次、冷却 60 秒、放行 5 个真实请求探测、3 个成功即恢复。你只需要选择作用范围并填写匹配对象。',
              )}
            />
            <Row gutter={16}>
              <Col span={12}>
                {renderRuleField(
                  t('规则名称'),
                  <Input
                    value={editingRule.name}
                    placeholder={t('例如：VIP 分组严格熔断')}
                    onChange={(value) =>
                      setEditingRule({ ...editingRule, name: value })
                    }
                  />,
                  t('用于在当前熔断状态表里识别命中的规则。'),
                )}
              </Col>
              <Col span={12}>
                {renderRuleField(
                  t('作用范围'),
                  <Select
                    value={editingRule.scope}
                    style={{ width: '100%' }}
                    onChange={(value) =>
                      setEditingRule({ ...editingRule, scope: value })
                    }
                  >
                    {scopeOptions.map((option) => (
                      <Select.Option key={option.value} value={option.value}>
                        {t(option.label)}
                      </Select.Option>
                    ))}
                  </Select>,
                )}
              </Col>
            </Row>
            {editingRule.scope !== 'global' && (
              <>
                {renderRuleField(
                  t('匹配对象'),
                  <TextArea
                    value={(editingRule.targets || []).join('\n')}
                    placeholder={t('一行一个，例如分组名、模型名或渠道 ID')}
                    autosize={{ minRows: 3, maxRows: 8 }}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        targets: String(value || '')
                          .split(/[\n,，]/)
                          .map((item) => item.trim())
                          .filter(Boolean),
                      })
                    }
                  />,
                  t(
                    '分组规则填写分组名，模型规则填写模型名，渠道规则填写渠道 ID；全局规则不需要匹配对象。',
                  ),
                )}
              </>
            )}
            <Row gutter={16}>
              <Col xs={24} sm={12} md={6}>
                {renderRuleField(
                  t('连续失败次数'),
                  <InputNumber
                    value={editingRule.failure_limit}
                    min={1}
                    style={{ width: '100%' }}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        failure_limit: Number(value || 1),
                      })
                    }
                  />,
                  t('默认 5。连续失败达到该次数后打开熔断。'),
                )}
              </Col>
              <Col xs={24} sm={12} md={6}>
                {renderRuleField(
                  t('冷却时间'),
                  <InputNumber
                    value={editingRule.cooldown_seconds}
                    min={1}
                    suffix={t('秒')}
                    style={{ width: '100%' }}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        cooldown_seconds: Number(value || 1),
                      })
                    }
                  />,
                  t('默认 60 秒。冷却结束后才允许真实请求探测。'),
                )}
              </Col>
              <Col xs={24} sm={12} md={6}>
                {renderRuleField(
                  t('探测请求数'),
                  <InputNumber
                    value={editingRule.probe_count}
                    min={1}
                    style={{ width: '100%' }}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        probe_count: Number(value || 1),
                      })
                    }
                  />,
                  t('默认 5。半开状态最多放行的真实请求数量。'),
                )}
              </Col>
              <Col xs={24} sm={12} md={6}>
                {renderRuleField(
                  t('探测成功要求'),
                  <InputNumber
                    value={editingRule.probe_success_count}
                    min={1}
                    max={editingRule.probe_count}
                    style={{ width: '100%' }}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        probe_success_count: Number(value || 1),
                      })
                    }
                  />,
                  t('默认 3。达到该成功数后恢复正常调度。'),
                )}
              </Col>
            </Row>
            {renderRuleField(
              t('失败状态码'),
              <Input
                value={editingRule.failure_status_codes}
                placeholder='429,500-599'
                onChange={(value) =>
                  setEditingRule({
                    ...editingRule,
                    failure_status_codes: value,
                  })
                }
              />,
              t('默认 429,500-599。只统计这些状态码为熔断失败。'),
            )}
            <Row gutter={16}>
              <Col xs={24} sm={12}>
                {renderRuleField(
                  t('失败关键词'),
                  <TextArea
                    value={editingRule.failure_keywords}
                    placeholder={t('一行一个，例如 rate limit、overloaded')}
                    autosize={{ minRows: 4, maxRows: 8 }}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        failure_keywords: value,
                      })
                    }
                  />,
                  t('上游错误文本命中这些关键词时，计入熔断失败。'),
                )}
              </Col>
              <Col xs={24} sm={12}>
                {renderRuleField(
                  t('熔断排除路径'),
                  <TextArea
                    value={editingRule.exclude_paths}
                    placeholder={'/v1/videos'}
                    autosize={{ minRows: 4, maxRows: 8 }}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        exclude_paths: value,
                      })
                    }
                  />,
                  t('一行一个路径前缀，命中后不计入熔断。'),
                )}
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={8}>
                {renderRuleField(
                  t('启用规则'),
                  <Switch
                    checked={editingRule.enabled}
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={(value) =>
                      setEditingRule({ ...editingRule, enabled: value })
                    }
                  />,
                  t('关闭后保留配置但不参与匹配。'),
                )}
              </Col>
              <Col xs={24} sm={8}>
                {renderRuleField(
                  t('不参与熔断'),
                  <Switch
                    checked={editingRule.disable_breaker}
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        disable_breaker: value,
                      })
                    }
                  />,
                  t('命中该规则后完全跳过熔断，适合视频或异步任务。'),
                )}
              </Col>
              <Col xs={24} sm={8}>
                {renderRuleField(
                  t('忽略 4xx 参数错误'),
                  <Switch
                    checked={editingRule.ignore_client_error_4xx}
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        ignore_client_error_4xx: value,
                      })
                    }
                  />,
                  t('开启后 400-499 不计入熔断失败，401/403 等也会被忽略。'),
                )}
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={8}>
                {renderRuleField(
                  t('立即禁用渠道'),
                  <Switch
                    checked={editingRule.instant_disable_enabled}
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        instant_disable_enabled: value,
                      })
                    }
                  />,
                  t(
                    '命中（状态码 AND 关键词）时直接永久禁用整个渠道，不走失败计数。典型场景：上游账号余额耗尽。',
                  ),
                )}
              </Col>
              <Col xs={24} sm={8}>
                {renderRuleField(
                  t('立即禁用状态码'),
                  <Input
                    value={editingRule.instant_disable_status_codes}
                    placeholder='403'
                    disabled={!editingRule.instant_disable_enabled}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        instant_disable_status_codes: value,
                      })
                    }
                  />,
                  t('与关键词同时命中才触发，例如 403。'),
                )}
              </Col>
              <Col xs={24} sm={8}>
                {renderRuleField(
                  t('立即禁用关键词'),
                  <TextArea
                    value={editingRule.instant_disable_keywords}
                    placeholder={
                      'insufficient account balance\ninsufficient_user_quota\n预扣费额度失败'
                    }
                    disabled={!editingRule.instant_disable_enabled}
                    autosize={{ minRows: 4, maxRows: 8 }}
                    onChange={(value) =>
                      setEditingRule({
                        ...editingRule,
                        instant_disable_keywords: value,
                      })
                    }
                  />,
                  t('一行一个，需与状态码同时命中。我方用户额度不足不会误伤。'),
                )}
              </Col>
            </Row>
          </div>
        )}
      </Modal>
    </Spin>
  );
}
