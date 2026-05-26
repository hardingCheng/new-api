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
  Modal,
  Radio,
  RadioGroup,
  Space,
  Table,
  Tag,
  TextArea,
} from '@douyinfe/semi-ui';
import { IconDelete, IconEdit, IconPlus, IconSave } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

const MODE_PER_SECOND = 'per_second';
const MODE_PER_CALL = 'per_call';

const parseRules = (raw) => {
  if (!raw || raw.trim() === '') {
    return [];
  }
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return [];
    }
    return Object.entries(parsed)
      .filter(([, mode]) => mode === MODE_PER_SECOND || mode === MODE_PER_CALL)
      .map(([model, mode]) => ({ model, mode }))
      .sort((a, b) => a.model.localeCompare(b.model));
  } catch (error) {
    return [];
  }
};

const buildRawValue = (rules) => {
  const map = {};
  for (const rule of rules) {
    const model = rule.model.trim();
    if (model && (rule.mode === MODE_PER_SECOND || rule.mode === MODE_PER_CALL)) {
      map[model] = rule.mode;
    }
  }
  return JSON.stringify(map, null, 2);
};

const getModeText = (mode, t) =>
  mode === MODE_PER_CALL ? t('按次计费') : t('按秒计费');

const getDefaultModeText = (t) => t('默认按秒计费');

const wildcardMatch = (pattern, value) => {
  const cleanPattern = pattern.trim();
  const cleanValue = value.trim();
  if (!cleanPattern || !cleanValue) {
    return false;
  }
  if (cleanPattern === cleanValue) {
    return true;
  }
  if (!cleanPattern.includes('*')) {
    return false;
  }
  const parts = cleanPattern.split('*');
  let position = 0;
  for (let index = 0; index < parts.length; index += 1) {
    const part = parts[index];
    if (!part) {
      continue;
    }
    const found = cleanValue.slice(position).indexOf(part);
    if (found < 0) {
      return false;
    }
    if (index === 0 && !cleanPattern.startsWith('*') && found !== 0) {
      return false;
    }
    position += found + part.length;
  }
  const lastPart = parts[parts.length - 1];
  return !lastPart || cleanPattern.endsWith('*') || cleanValue.endsWith(lastPart);
};

const findMatchedRule = (rules, modelName) => {
  const model = modelName.trim();
  if (!model) {
    return null;
  }
  const exact = rules.find((rule) => rule.model === model);
  if (exact) {
    return { ...exact, matchType: 'exact' };
  }
  return rules
    .filter((rule) => wildcardMatch(rule.model, model))
    .sort(
      (a, b) =>
        b.model.replaceAll('*', '').length - a.model.replaceAll('*', '').length,
    )
    .map((rule) => ({ ...rule, matchType: 'wildcard' }))[0] || null;
};

export default function VideoBillingModeSettings({ options, refresh }) {
  const { t } = useTranslation();
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingModel, setEditingModel] = useState('');
  const [formState, setFormState] = useState({
    model: '',
    mode: MODE_PER_SECOND,
  });
  const [previewModel, setPreviewModel] = useState('');

  useEffect(() => {
    setRules(parseRules(options.VideoBillingMode));
  }, [options.VideoBillingMode]);

  const rawValue = useMemo(() => buildRawValue(rules), [rules]);
  const previewMatch = useMemo(
    () => findMatchedRule(rules, previewModel),
    [previewModel, rules],
  );

  const openCreateModal = () => {
    setEditingModel('');
    setFormState({ model: '', mode: MODE_PER_SECOND });
    setModalVisible(true);
  };

  const openEditModal = (rule) => {
    setEditingModel(rule.model);
    setFormState({ model: rule.model, mode: rule.mode });
    setModalVisible(true);
  };

  const upsertRule = () => {
    const model = formState.model.trim();
    if (!model) {
      showError(t('请输入模型名称'));
      return;
    }

    setRules((previous) => {
      const next = previous.filter((rule) => rule.model !== editingModel && rule.model !== model);
      return [...next, { model, mode: formState.mode }].sort((a, b) =>
        a.model.localeCompare(b.model),
      );
    });
    setModalVisible(false);
  };

  const deleteRule = (model) => {
    setRules((previous) => previous.filter((rule) => rule.model !== model));
  };

  const saveRules = async () => {
    setLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'VideoBillingMode',
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
      title: t('模型匹配规则'),
      dataIndex: 'model',
      render: (text) => <Tag shape='circle'>{text}</Tag>,
    },
    {
      title: t('计费模式'),
      dataIndex: 'mode',
      render: (mode) => (
        <Tag color={mode === MODE_PER_CALL ? 'teal' : 'blue'} shape='circle'>
          {getModeText(mode, t)}
        </Tag>
      ),
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
            onClick={() => deleteRule(record.model)}
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
            {t('保存视频计费模式')}
          </Button>
        </Space>
        <div className='mt-3 text-sm text-gray-500'>
          {t(
            '仅影响 /v1/videos 接口。按秒计费 = 生成秒数 * 模型单价；按次计费 = 不管秒数直接按模型单价收费。模型名支持 seedance-* 这类通配符。',
          )}
        </div>
      </Card>

      <Card title={t('规则命中预览')} style={{ width: '100%' }}>
        <Space vertical align='start' style={{ width: '100%' }}>
          <Input
            value={previewModel}
            placeholder='seedance-2.0-480p'
            onChange={setPreviewModel}
            style={{ maxWidth: 420 }}
            showClear
          />
          {previewModel.trim() ? (
            previewMatch ? (
              <Space wrap>
                <Tag shape='circle' color='blue'>
                  {t('命中规则')}：{previewMatch.model}
                </Tag>
                <Tag shape='circle' color={previewMatch.mode === MODE_PER_CALL ? 'teal' : 'blue'}>
                  {t('计费模式')}：{getModeText(previewMatch.mode, t)}
                </Tag>
                <Tag shape='circle'>
                  {previewMatch.matchType === 'exact'
                    ? t('精确匹配')
                    : t('通配符匹配')}
                </Tag>
              </Space>
            ) : (
              <Tag shape='circle' color='grey'>
                {t('未命中配置规则')}：{getDefaultModeText(t)}
              </Tag>
            )
          ) : (
            <div className='text-sm text-gray-500'>
              {t('输入一个视频模型名，预览最终会命中的计费模式。')}
            </div>
          )}
        </Space>
      </Card>

      <Card bodyStyle={{ padding: 0 }} style={{ width: '100%' }}>
        <Table
          columns={columns}
          dataSource={rules}
          rowKey='model'
          pagination={false}
          empty={<Empty title={t('暂无视频计费规则')} />}
        />
      </Card>

      <Modal
        title={editingModel ? t('编辑视频计费规则') : t('新增视频计费规则')}
        visible={modalVisible}
        onOk={upsertRule}
        onCancel={() => setModalVisible(false)}
        okText={t('确认')}
        cancelText={t('取消')}
      >
        <Form layout='vertical'>
          <Form.Input
            field='model'
            label={t('模型匹配规则')}
            placeholder='seedance-*'
            initValue={formState.model}
            onChange={(value) => setFormState({ ...formState, model: value })}
          />
          <div className='mb-2 font-medium text-gray-700'>{t('计费模式')}</div>
          <RadioGroup
            type='button'
            value={formState.mode}
            onChange={(event) =>
              setFormState({ ...formState, mode: event.target.value })
            }
          >
            <Radio value={MODE_PER_SECOND}>{t('按秒计费')}</Radio>
            <Radio value={MODE_PER_CALL}>{t('按次计费')}</Radio>
          </RadioGroup>
          <TextArea
            className='mt-4'
            readonly
            autosize={{ minRows: 4, maxRows: 8 }}
            value={rawValue}
          />
        </Form>
      </Modal>
    </Space>
  );
}
