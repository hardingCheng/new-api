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

import {
  TASK_ACTION_FIRST_TAIL_GENERATE,
  TASK_ACTION_GENERATE,
  TASK_ACTION_REFERENCE_GENERATE,
  TASK_ACTION_TEXT_GENERATE,
  TASK_ACTION_REMIX_GENERATE,
} from '../../../constants/common.constant';
import { CHANNEL_OPTIONS } from '../../../constants/channel.constants';
import { quotaToDisplayAmount } from '../../../helpers/quota';

export const EXPORT_PAGE_SIZE = 100;

const pad = (n) => ('0' + n).slice(-2);

const formatTimestamp = (timestampInSeconds) => {
  if (!timestampInSeconds) return '';
  const date = new Date(timestampInSeconds * 1000);
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  );
};

// 导出用：返回全精度数值（不逐行四舍五入），保证 Excel 求和与后端按原始额度汇总后换算的结果一致
const toQuotaNumber = (quota) => {
  const amount = quotaToDisplayAmount(quota);
  if (!Number.isFinite(amount)) {
    return 0;
  }
  return amount;
};

const getModelName = (record) =>
  record?.properties?.origin_model_name ||
  record?.properties?.upstream_model_name ||
  record?.data?.model ||
  '';

const getVideoDurationSeconds = (record) => {
  const data = record?.data;
  if (!data || typeof data !== 'object' || Array.isArray(data)) {
    return null;
  }
  const raw =
    data.duration ??
    data.seconds ??
    data.metadata?.duration ??
    data.metadata?.seconds ??
    data.metadata?.durationSeconds;
  if (raw === undefined || raw === null || raw === '') {
    return null;
  }
  const value = Number(raw);
  return Number.isFinite(value) && value > 0 ? value : null;
};

const getActionLabel = (action, t) => {
  switch (action) {
    case 'MUSIC':
      return t('生成音乐');
    case 'LYRICS':
      return t('生成歌词');
    case TASK_ACTION_GENERATE:
      return t('图生视频');
    case TASK_ACTION_TEXT_GENERATE:
      return t('文生视频');
    case TASK_ACTION_FIRST_TAIL_GENERATE:
      return t('首尾生视频');
    case TASK_ACTION_REFERENCE_GENERATE:
      return t('参照生视频');
    case TASK_ACTION_REMIX_GENERATE:
      return t('视频Remix');
    default:
      return t('未知');
  }
};

const getStatusLabel = (status, t) => {
  switch (status) {
    case 'SUCCESS':
      return t('成功');
    case 'NOT_START':
      return t('未启动');
    case 'SUBMITTED':
      return t('队列中');
    case 'IN_PROGRESS':
      return t('执行中');
    case 'FAILURE':
      return t('失败');
    case 'QUEUED':
      return t('排队中');
    case '':
      return t('正在提交');
    default:
      return t('未知');
  }
};

const getPlatformLabel = (platform, t) => {
  const option = CHANNEL_OPTIONS.find(
    (opt) => String(opt.value) === String(platform),
  );
  if (option) {
    return option.label;
  }
  if (platform === 'suno') {
    return 'Suno';
  }
  return platform || t('未知');
};

export const buildTaskExportRows = (items, { t, isAdminUser }) => {
  return items.map((record) => {
    const submit = record.submit_time;
    const finish = record.finish_time;
    const durationSec = submit && finish ? finish - submit : '';
    const refundQuota =
      record.refund_quota ||
      (record.status === 'FAILURE' ? record.quota || 0 : 0);
    const videoDuration = getVideoDurationSeconds(record);
    const hasReference = !!record?.properties?.has_reference_video;
    const referenceSeconds = record?.properties?.reference_video_seconds || 0;

    const row = {
      [t('提交时间')]: formatTimestamp(submit),
      [t('结束时间')]: formatTimestamp(finish),
      [t('花费时间') + '(s)']: durationSec,
    };
    if (isAdminUser) {
      row[t('渠道名称')] = record.channel_name || '';
      row[t('渠道 ID')] = record.channel_id ?? '';
      row[t('用户')] = record.username || '';
    }
    row[t('平台')] = getPlatformLabel(record.platform, t);
    row[t('模型')] = getModelName(record);
    if (isAdminUser) {
      row[t('消耗额度')] = toQuotaNumber(record.quota || 0);
      row[t('退款额度')] = refundQuota > 0 ? toQuotaNumber(refundQuota) : 0;
    }
    row[t('视频时长') + '(s)'] = videoDuration || '';
    if (isAdminUser) {
      row[t('是否视频参考')] = hasReference ? t('是') : t('否');
      row[t('参考视频时长') + '(s)'] = referenceSeconds || '';
    }
    row[t('类型')] = getActionLabel(record.action, t);
    row[t('任务ID')] = record.task_id || '';
    row[t('任务状态')] = getStatusLabel(record.status, t);
    row[t('进度')] = record.progress || '';
    row[t('详情')] = record.fail_reason || '';
    return row;
  });
};
