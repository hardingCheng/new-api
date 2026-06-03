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
import { Banner, Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import {
  API,
  compareObjects,
  showError,
  showSuccess,
  showWarning,
  toBoolean,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const OPTION_KEYS = [
  'model_limit_setting.seedance_resource_pool_guard_enabled',
  'model_limit_setting.seedance_resource_pool_guard_models',
  'model_limit_setting.seedance_resource_pool_guard_user_ids',
  'model_limit_setting.seedance_resource_pool_guard_message',
];

const defaultInputs = {
  'model_limit_setting.seedance_resource_pool_guard_enabled': true,
  'model_limit_setting.seedance_resource_pool_guard_models':
    'seedance-2.0-fast-480p',
  'model_limit_setting.seedance_resource_pool_guard_user_ids': '42\n2113417732',
  'model_limit_setting.seedance_resource_pool_guard_message':
    '此模型资源池已耗尽，请使用其他的模型。',
};

export default function SettingsModelLimit(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(defaultInputs);
  const [inputsRow, setInputsRow] = useState(defaultInputs);
  const refForm = useRef();

  function normalizeValue(value) {
    if (typeof value === 'boolean') {
      return String(value);
    }
    return String(value ?? '');
  }

  async function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    const requestQueue = updateArray.map((item) =>
      API.put('/api/option/', {
        key: item.key,
        value: normalizeValue(inputs[item.key]),
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

  useEffect(() => {
    const currentInputs = { ...defaultInputs };
    for (const key of OPTION_KEYS) {
      if (!Object.prototype.hasOwnProperty.call(props.options || {}, key)) {
        continue;
      }
      if (key === 'model_limit_setting.seedance_resource_pool_guard_enabled') {
        currentInputs[key] = toBoolean(props.options[key]);
      } else {
        currentInputs[key] = props.options[key] ?? defaultInputs[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    if (refForm.current) {
      refForm.current.setValues(currentInputs);
    }
  }, [props.options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('模型限制')}>
          <Banner
            fullMode={false}
            type='info'
            description={t(
              '当指定用户调用指定模型时直接拦截请求并返回提示，用于资源池耗尽等场景的临时限流。仅当模型与用户同时命中时才会拦截。',
            )}
          />
          <Row gutter={16} style={{ marginTop: 12 }}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field='model_limit_setting.seedance_resource_pool_guard_enabled'
                label={t('启用模型限制')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'model_limit_setting.seedance_resource_pool_guard_enabled':
                      value,
                  })
                }
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.TextArea
                field='model_limit_setting.seedance_resource_pool_guard_models'
                label={t('限制模型')}
                placeholder={'seedance-2.0-fast-480p'}
                autosize={{ minRows: 3, maxRows: 8 }}
                extraText={t(
                  '命中这些模型的请求会被拦截，支持换行、逗号、分号或空格分隔，不区分大小写。',
                )}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'model_limit_setting.seedance_resource_pool_guard_models':
                      value,
                  })
                }
              />
            </Col>
            <Col xs={24} sm={12}>
              <Form.TextArea
                field='model_limit_setting.seedance_resource_pool_guard_user_ids'
                label={t('限制用户 ID')}
                placeholder={'42\n2113417732'}
                autosize={{ minRows: 3, maxRows: 8 }}
                extraText={t(
                  '只有这些用户 ID 调用上述模型时才会被拦截，支持换行、逗号、分号或空格分隔。',
                )}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'model_limit_setting.seedance_resource_pool_guard_user_ids':
                      value,
                  })
                }
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={16}>
              <Form.Input
                field='model_limit_setting.seedance_resource_pool_guard_message'
                label={t('拦截提示语')}
                placeholder={'此模型资源池已耗尽，请使用其他的模型。'}
                extraText={t('命中限制时返回给用户的提示文本。')}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    'model_limit_setting.seedance_resource_pool_guard_message':
                      value,
                  })
                }
              />
            </Col>
          </Row>
          <Row>
            <Button size='default' onClick={onSubmit}>
              {t('保存模型限制设置')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
