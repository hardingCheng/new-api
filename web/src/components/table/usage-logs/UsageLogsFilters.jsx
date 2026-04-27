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

const LogsFilters = ({
  formInitValues,
  setFormApi,
  refresh,
  resetFilters,
  setShowColumnSelector,
  loading,
  isAdminUser,
  userOptions,
  userOptionsLoading,
  handleUserSearch,
  handleUserSelectionChange,
  handleUserDropdownVisibleChange,
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
        <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2'>
          <div className='col-span-1 lg:col-span-2'>
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

          <div>
            <Form.Select
              field='logType'
              placeholder={t('日志类型')}
              className='w-full'
              showClear
              pure
              onChange={() => {
                setTimeout(() => {
                  refresh();
                }, 0);
              }}
              size='small'
            >
              <Form.Select.Option value='0'>{t('全部')}</Form.Select.Option>
              <Form.Select.Option value='1'>{t('充值')}</Form.Select.Option>
              <Form.Select.Option value='2'>{t('消费')}</Form.Select.Option>
              <Form.Select.Option value='3'>{t('管理')}</Form.Select.Option>
              <Form.Select.Option value='4'>{t('系统')}</Form.Select.Option>
              <Form.Select.Option value='5'>{t('错误')}</Form.Select.Option>
              <Form.Select.Option value='6'>{t('退款')}</Form.Select.Option>
            </Form.Select>
          </div>

          <Form.Input
            field='token_name'
            prefix={<IconSearch />}
            placeholder={t('令牌名称')}
            showClear
            pure
            size='small'
          />

          <Form.Input
            field='model_name'
            prefix={<IconSearch />}
            placeholder={t('模型名称')}
            showClear
            pure
            size='small'
          />

          <Form.Input
            field='group'
            prefix={<IconSearch />}
            placeholder={t('分组')}
            showClear
            pure
            size='small'
          />

          <Form.Input
            field='request_id'
            prefix={<IconSearch />}
            placeholder={t('Request ID')}
            showClear
            pure
            size='small'
          />

          {isAdminUser && (
            <>
              <Form.Input
                field='channel'
                prefix={<IconSearch />}
                placeholder={t('渠道 ID')}
                showClear
                pure
                size='small'
              />

              <Form.Input
                field='username'
                prefix={<IconSearch />}
                placeholder={t('用户名')}
                showClear
                pure
                size='small'
              />

              <div className='col-span-1 md:col-span-2'>
                <Form.Select
                  field='usernames'
                  optionList={userOptions}
                  placeholder={t('输入用户名搜索')}
                  className='w-full'
                  multiple
                  filter
                  searchPosition='dropdown'
                  autoClearSearchValue={false}
                  showClear
                  pure
                  size='small'
                  loading={userOptionsLoading}
                  onSearch={handleUserSearch}
                  onChange={handleUserSelectionChange}
                  onDropdownVisibleChange={handleUserDropdownVisibleChange}
                  emptyContent={t('输入用户名搜索')}
                />
              </div>
            </>
          )}
        </div>

        <div className='flex flex-col sm:flex-row justify-end items-start sm:items-center gap-2'>
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
            onClick={resetFilters}
            htmlType='button'
            size='small'
          >
            {t('重置')}
          </Button>
          <Button
            type='tertiary'
            onClick={() => setShowColumnSelector(true)}
            htmlType='button'
            size='small'
          >
            {t('列设置')}
          </Button>
        </div>
      </div>
    </Form>
  );
};

export default LogsFilters;
