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
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import {
  API,
  compareObjects,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const seedanceSettingDefaults = {
  'task_billing_setting.seedance_reference_video_mode': 'legacy',
};

export default function SettingSeedanceModel(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(seedanceSettingDefaults);
  const [inputsRow, setInputsRow] = useState(seedanceSettingDefaults);
  const refForm = useRef();

  function handleFieldChange(fieldName) {
    return (value) => {
      setInputs((prev) => ({ ...prev, [fieldName]: value }));
    };
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) =>
      API.put('/api/option/', {
        key: item.key,
        value: String(inputs[item.key]),
      }),
    );

    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = { ...seedanceSettingDefaults };
    for (const key of Object.keys(seedanceSettingDefaults)) {
      if (props.options?.[key] !== undefined && props.options[key] !== '') {
        currentInputs[key] = props.options[key];
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
        <Form.Section text={t('Seedance设置')}>
          <Row>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Select
                field={'task_billing_setting.seedance_reference_video_mode'}
                label={t('Seedance 参考视频计费模式')}
                initValue={'legacy'}
                onChange={handleFieldChange(
                  'task_billing_setting.seedance_reference_video_mode',
                )}
                extraText={t(
                  'legacy: 保持原逻辑；duration: reference_video=1，并把参考视频秒数加到 seconds',
                )}
                optionList={[
                  {
                    label: t('legacy'),
                    value: 'legacy',
                  },
                  {
                    label: t('duration'),
                    value: 'duration',
                  },
                ]}
              />
            </Col>
          </Row>
          <Row>
            <Button size='default' onClick={onSubmit}>
              {t('保存')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
