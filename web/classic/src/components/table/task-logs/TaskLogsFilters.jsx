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

import React from 'react';
import { Button, Form } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';

import { DATE_RANGE_PRESETS } from '../../../constants/console.constants';

const TASK_STATUS_OPTIONS = [
  { label: 'NOT_START', value: 'NOT_START' },
  { label: 'SUBMITTED', value: 'SUBMITTED' },
  { label: 'QUEUED', value: 'QUEUED' },
  { label: 'IN_PROGRESS', value: 'IN_PROGRESS' },
  { label: 'SUCCESS', value: 'SUCCESS' },
  { label: 'FAILURE', value: 'FAILURE' },
  { label: 'UNKNOWN', value: 'UNKNOWN' },
];

const TaskLogsFilters = ({
  formInitValues,
  setFormApi,
  refresh,
  setShowColumnSelector,
  formApi,
  loading,
  isAdminUser,
  userOptions,
  modelOptions,
  t,
}) => {
  return (
    <Form
      initValues={formInitValues}
      getFormApi={(api) => setFormApi(api)}
      onSubmit={refresh}
      allowEmpty={true}
      autoComplete='off'
      layout='vertical'
      trigger='change'
      stopValidateWithError={false}
    >
      <div className='flex flex-col gap-2'>
        <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-6 gap-2'>
          {/* 时间选择器 */}
          <div className='col-span-1 md:col-span-2'>
            <Form.DatePicker
              field='dateRange'
              className='w-full'
              type='dateTimeRange'
              placeholder={[t('开始时间'), t('结束时间')]}
              showClear
              pure
              size='small'
              presets={DATE_RANGE_PRESETS.map((preset) => ({
                text: t(preset.text),
                start: preset.start(),
                end: preset.end(),
              }))}
            />
          </div>

          {/* 任务 ID */}
          <Form.Input
            field='task_id'
            prefix={<IconSearch />}
            placeholder={t('任务 ID')}
            showClear
            pure
            size='small'
          />

          {/* 渠道 ID - 仅管理员可见 */}
          {isAdminUser && (
            <Form.Input
              field='channel_id'
              prefix={<IconSearch />}
              placeholder={t('渠道 ID')}
              showClear
              pure
              size='small'
            />
          )}

          {/* 用户筛选 - 仅管理员可见 */}
          {isAdminUser && (
            <Form.Select
              field='user_ids'
              placeholder={t('用户筛选')}
              optionList={userOptions}
              multiple
              filter
              allowCreate
              showClear
              pure
              size='small'
              maxTagCount={1}
              showRestTagsPopover
            />
          )}

          {/* 模型名称 - 仅管理员可见 */}
          {isAdminUser && (
            <Form.Select
              field='model_names'
              placeholder={t('模型名称')}
              optionList={modelOptions}
              multiple
              filter
              allowCreate
              showClear
              pure
              size='small'
              maxTagCount={1}
              showRestTagsPopover
            />
          )}

          {/* 任务状态 */}
          <Form.Select
            field='status'
            placeholder={t('任务状态')}
            optionList={[
              { label: t('所有状态'), value: '' },
              ...TASK_STATUS_OPTIONS,
            ]}
            showClear
            pure
            size='small'
          />

          {/* 视频参考 - 仅管理员可见 */}
          {isAdminUser && (
            <Form.Select
              field='reference'
              placeholder={t('视频参考')}
              optionList={[
                { label: t('有视频参考'), value: 'with' },
                { label: t('无视频参考'), value: 'without' },
              ]}
              showClear
              pure
              size='small'
            />
          )}
        </div>

        {/* 操作按钮区域 */}
        <div className='flex justify-between items-center'>
          <div></div>
          <div className='flex gap-2'>
            <Button
              type='tertiary'
              htmlType='submit'
              loading={loading}
              size='small'
            >
              {t('查询')}
            </Button>
            <Button
              type='tertiary'
              onClick={() => {
                if (formApi) {
                  formApi.reset();
                  // 重置后立即查询，使用setTimeout确保表单重置完成
                  setTimeout(() => {
                    refresh();
                  }, 100);
                }
              }}
              size='small'
            >
              {t('重置')}
            </Button>
            <Button
              type='tertiary'
              onClick={() => setShowColumnSelector(true)}
              size='small'
            >
              {t('列设置')}
            </Button>
          </div>
        </div>
      </div>
    </Form>
  );
};

export default TaskLogsFilters;
