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
  clearPersistentUsernames,
  persistentUsernames,
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
      <div className='rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3'>
        <div className='grid grid-cols-1 md:grid-cols-6 xl:grid-cols-12 gap-3'>
          <div className='md:col-span-3 xl:col-span-4'>
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

          <div className='md:col-span-3 xl:col-span-2'>
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

          <div className='md:col-span-3 xl:col-span-2'>
            <Form.Input
              field='token_name'
              prefix={<IconSearch />}
              placeholder={t('令牌名称')}
              showClear
              pure
              size='small'
            />
          </div>

          <div className='md:col-span-3 xl:col-span-2'>
            <Form.Input
              field='model_name'
              prefix={<IconSearch />}
              placeholder={t('模型名称')}
              showClear
              pure
              size='small'
            />
          </div>

          <div className='md:col-span-3 xl:col-span-2'>
            <Form.Input
              field='group'
              prefix={<IconSearch />}
              placeholder={t('分组')}
              showClear
              pure
              size='small'
            />
          </div>

          <div className='md:col-span-3 xl:col-span-3'>
            <Form.Input
              field='request_id'
              prefix={<IconSearch />}
              placeholder={t('Request ID')}
              showClear
              pure
              size='small'
            />
          </div>

          {isAdminUser && (
            <>
              <div className='md:col-span-3 xl:col-span-2'>
                <Form.Input
                  field='channel'
                  prefix={<IconSearch />}
                  placeholder={t('渠道 ID')}
                  showClear
                  pure
                  size='small'
                />
              </div>

              <div className='md:col-span-3 xl:col-span-2'>
                <Form.Input
                  field='username'
                  prefix={<IconSearch />}
                  placeholder={t('用户名')}
                  showClear
                  pure
                  size='small'
                />
              </div>

              <div className='md:col-span-6 xl:col-span-7 rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-1)] p-3'>
                <div className='flex items-center justify-between gap-2 mb-2 text-xs text-[var(--semi-color-text-2)]'>
                  <span>{t('选择用户（可多选）')}</span>
                  {persistentUsernames.length > 0 && (
                    <Button
                      type='tertiary'
                      size='small'
                      htmlType='button'
                      onClick={clearPersistentUsernames}
                    >
                      {t('清空')}
                    </Button>
                  )}
                </div>
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
                <div className='mt-2 text-xs text-[var(--semi-color-text-2)] leading-5'>
                  {t('多选用户会保留在本地，刷新后继续生效')}
                </div>
              </div>
            </>
          )}
        </div>

        <div className='mt-3 flex flex-wrap justify-end gap-2 border-t border-[var(--semi-color-border)] pt-3'>
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
