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

import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@douyinfe/semi-ui';
import {
  API,
  getTodayStartTimestamp,
  isAdmin,
  showError,
  showSuccess,
  timestamp2string,
  renderQuota,
  renderNumber,
  getLogOther,
  copy,
  renderClaudeLogContent,
  renderLogContent,
  renderAudioModelPrice,
  renderClaudeModelPrice,
  renderModelPrice,
} from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';

export const useLogsData = () => {
  const { t } = useTranslation();

  const formatLogSeconds = (value) => {
    const seconds = Number(value);
    if (!Number.isFinite(seconds) || seconds <= 0) {
      return '';
    }
    return seconds.toFixed(2);
  };

  // Define column keys for selection
  const COLUMN_KEYS = {
    TIME: 'time',
    CHANNEL: 'channel',
    USERNAME: 'username',
    TOKEN: 'token',
    GROUP: 'group',
    TYPE: 'type',
    MODEL: 'model',
    USE_TIME: 'use_time',
    PROMPT: 'prompt',
    COMPLETION: 'completion',
    COST: 'cost',
    RETRY: 'retry',
    IP: 'ip',
    DETAILS: 'details',
  };

  // Basic state
  const [logs, setLogs] = useState([]);
  const [expandData, setExpandData] = useState({});
  const [showStat, setShowStat] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingStat, setLoadingStat] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [logCount, setLogCount] = useState(0);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [logType, setLogType] = useState(0);

  // User and admin
  const isAdminUser = isAdmin();
  // Role-specific storage key to prevent different roles from overwriting each other
  const STORAGE_KEY = isAdminUser
    ? 'logs-table-columns-admin'
    : 'logs-table-columns-user';
  const ADMIN_USER_FILTER_STORAGE_KEY = 'logs-filter-admin-selected-usernames';

  // Statistics state
  const [stat, setStat] = useState({
    quota: 0,
    token: 0,
  });

  const normalizeUsernames = (values) => {
    if (!Array.isArray(values)) {
      return [];
    }

    const deduped = new Set();
    values.forEach((value) => {
      const username = `${value || ''}`.trim();
      if (username) {
        deduped.add(username);
      }
    });

    return Array.from(deduped);
  };

  const loadPersistentUsernames = () => {
    if (!isAdminUser) {
      return [];
    }
    try {
      const saved = localStorage.getItem(ADMIN_USER_FILTER_STORAGE_KEY);
      if (!saved) {
        return [];
      }
      return normalizeUsernames(JSON.parse(saved));
    } catch (error) {
      console.error('Failed to parse saved admin log usernames', error);
      return [];
    }
  };

  // Form state
  const [formApi, setFormApi] = useState(null);
  const [persistentUsernames, setPersistentUsernames] = useState(
    loadPersistentUsernames,
  );
  const initialDateRangeRef = useRef([
    timestamp2string(getTodayStartTimestamp()),
    timestamp2string(Date.now() / 1000 + 3600),
  ]);

  const getDefaultFormInitValues = (savedUsernames = persistentUsernames) => ({
    username: '',
    usernames: normalizeUsernames(savedUsernames),
    token_name: '',
    model_name: '',
    channel: '',
    group: '',
    request_id: '',
    dateRange: [...initialDateRangeRef.current],
    logType: '0',
  });

  const formInitValues = getDefaultFormInitValues();

  // Column visibility state
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);

  // Compact mode
  const [compactMode, setCompactMode] = useTableCompactMode('logs');

  // User info modal state
  const [showUserInfo, setShowUserInfoModal] = useState(false);
  const [userInfoData, setUserInfoData] = useState(null);
  const [userOptions, setUserOptions] = useState([]);
  const [userOptionsLoading, setUserOptionsLoading] = useState(false);
  const userSearchTimeoutRef = useRef(null);
  const latestUserSearchRef = useRef(0);

  // Channel affinity usage cache stats modal state (admin only)
  const [
    showChannelAffinityUsageCacheModal,
    setShowChannelAffinityUsageCacheModal,
  ] = useState(false);
  const [channelAffinityUsageCacheTarget, setChannelAffinityUsageCacheTarget] =
    useState(null);

  // Load saved column preferences from localStorage
  useEffect(() => {
    const savedColumns = localStorage.getItem(STORAGE_KEY);
    if (savedColumns) {
      try {
        const parsed = JSON.parse(savedColumns);
        const defaults = getDefaultColumnVisibility();
        const merged = { ...defaults, ...parsed };

        // For non-admin users, force-hide admin-only columns (does not touch admin settings)
        if (!isAdminUser) {
          merged[COLUMN_KEYS.CHANNEL] = false;
          merged[COLUMN_KEYS.USERNAME] = false;
          merged[COLUMN_KEYS.RETRY] = false;
        }
        setVisibleColumns(merged);
      } catch (e) {
        console.error('Failed to parse saved column preferences', e);
        initDefaultColumns();
      }
    } else {
      initDefaultColumns();
    }
  }, []);

  useEffect(() => {
    if (!isAdminUser) {
      return;
    }

    setUserOptions((prev) => mergeUserOptions(prev, [], persistentUsernames));
  }, [isAdminUser, persistentUsernames]);

  // Get default column visibility based on user role
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.TIME]: true,
      [COLUMN_KEYS.CHANNEL]: isAdminUser,
      [COLUMN_KEYS.USERNAME]: isAdminUser,
      [COLUMN_KEYS.TOKEN]: true,
      [COLUMN_KEYS.GROUP]: true,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.MODEL]: true,
      [COLUMN_KEYS.USE_TIME]: true,
      [COLUMN_KEYS.PROMPT]: true,
      [COLUMN_KEYS.COMPLETION]: true,
      [COLUMN_KEYS.COST]: true,
      [COLUMN_KEYS.RETRY]: isAdminUser,
      [COLUMN_KEYS.IP]: true,
      [COLUMN_KEYS.DETAILS]: true,
    };
  };

  // Initialize default column visibility
  const initDefaultColumns = () => {
    const defaults = getDefaultColumnVisibility();
    setVisibleColumns(defaults);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(defaults));
  };

  // Handle column visibility change
  const handleColumnVisibilityChange = (columnKey, checked) => {
    const updatedColumns = { ...visibleColumns, [columnKey]: checked };
    setVisibleColumns(updatedColumns);
  };

  // Handle "Select All" checkbox
  const handleSelectAll = (checked) => {
    const allKeys = Object.keys(COLUMN_KEYS).map((key) => COLUMN_KEYS[key]);
    const updatedColumns = {};

    allKeys.forEach((key) => {
      if (
        (key === COLUMN_KEYS.CHANNEL ||
          key === COLUMN_KEYS.USERNAME ||
          key === COLUMN_KEYS.RETRY) &&
        !isAdminUser
      ) {
        updatedColumns[key] = false;
      } else {
        updatedColumns[key] = checked;
      }
    });

    setVisibleColumns(updatedColumns);
  };

  // Persist column settings to the role-specific STORAGE_KEY
  useEffect(() => {
    if (Object.keys(visibleColumns).length > 0) {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(visibleColumns));
    }
  }, [visibleColumns]);

  // 获取表单值的辅助函数，确保所有值都是字符串
  const getFormValues = () => {
    const fallbackValues = getDefaultFormInitValues();
    const formValues = formApi ? formApi.getValues() : fallbackValues;
    const username = `${formValues.username || ''}`.trim();
    const usernames = normalizeUsernames([
      ...(Array.isArray(formValues.usernames)
        ? formValues.usernames
        : fallbackValues.usernames),
      username,
    ]);

    let [start_timestamp, end_timestamp] = fallbackValues.dateRange;

    if (
      formValues.dateRange &&
      Array.isArray(formValues.dateRange) &&
      formValues.dateRange.length === 2
    ) {
      start_timestamp = formValues.dateRange[0];
      end_timestamp = formValues.dateRange[1];
    }

    return {
      username,
      usernames,
      token_name: formValues.token_name || '',
      model_name: formValues.model_name || '',
      start_timestamp,
      end_timestamp,
      channel: formValues.channel || '',
      group: formValues.group || '',
      request_id: formValues.request_id || '',
      logType: formValues.logType ? parseInt(formValues.logType) : 0,
    };
  };

  const mergeUserOptions = (
    currentOptions = [],
    fetchedUsers = [],
    selectedUsernames = [],
  ) => {
    const optionMap = new Map();

    currentOptions.forEach((option) => {
      if (option?.value) {
        optionMap.set(option.value, option);
      }
    });

    fetchedUsers.forEach((user) => {
      if (!user?.username) {
        return;
      }
      const label =
        user.display_name && user.display_name !== user.username
          ? `${user.username} (${user.display_name})`
          : user.username;
      optionMap.set(user.username, {
        label,
        value: user.username,
        pinned: Boolean(user.pinned),
      });
    });

    selectedUsernames.forEach((username) => {
      if (!username || optionMap.has(username)) {
        return;
      }
      optionMap.set(username, {
        label: username,
        value: username,
        pinned: false,
      });
    });

    return Array.from(optionMap.values()).sort((a, b) => {
      if (Boolean(a.pinned) !== Boolean(b.pinned)) {
        return a.pinned ? -1 : 1;
      }
      return String(a.label || a.value).localeCompare(
        String(b.label || b.value),
        'zh-Hans-CN',
      );
    });
  };

  const fetchUserOptions = async (keyword = '') => {
    if (!isAdminUser) {
      return;
    }

    const requestId = latestUserSearchRef.current + 1;
    latestUserSearchRef.current = requestId;
    setUserOptionsLoading(true);

    const trimmedKeyword = keyword.trim();
    const url = trimmedKeyword
      ? `/api/user/search?keyword=${encodeURIComponent(trimmedKeyword)}&group=&p=1&page_size=100`
      : '/api/user/?p=1&page_size=100';

    try {
      const res = await API.get(url, { disableDuplicate: true });
      const { success, message, data } = res.data;
      if (!success) {
        if (latestUserSearchRef.current === requestId) {
          showError(message);
        }
        return;
      }

      if (latestUserSearchRef.current !== requestId) {
        return;
      }

      const selectedUsernames =
        formApi?.getValues()?.usernames || persistentUsernames;
      setUserOptions((prev) =>
        mergeUserOptions(prev, data?.items || [], selectedUsernames),
      );
    } catch {
      // Global API interceptor already handles user-facing errors.
    } finally {
      if (latestUserSearchRef.current === requestId) {
        setUserOptionsLoading(false);
      }
    }
  };

  const handleUserSearch = (keyword) => {
    if (!isAdminUser) {
      return;
    }

    if (userSearchTimeoutRef.current) {
      clearTimeout(userSearchTimeoutRef.current);
    }

    userSearchTimeoutRef.current = setTimeout(() => {
      fetchUserOptions(keyword);
    }, 250);
  };

  const handleUserSelectionChange = (selectedUsernames) => {
    const normalizedUsernames = normalizeUsernames(selectedUsernames || []);
    setPersistentUsernames(normalizedUsernames);
    localStorage.setItem(
      ADMIN_USER_FILTER_STORAGE_KEY,
      JSON.stringify(normalizedUsernames),
    );
    setUserOptions((prev) => mergeUserOptions(prev, [], normalizedUsernames));
  };

  const handleUserDropdownVisibleChange = (visible) => {
    if (visible && userOptions.length === 0) {
      fetchUserOptions('');
    }
  };

  const clearPersistentUsernames = () => {
    if (!isAdminUser) {
      return;
    }

    setPersistentUsernames([]);
    localStorage.removeItem(ADMIN_USER_FILTER_STORAGE_KEY);
    if (formApi) {
      formApi.setValue('usernames', []);
    }
    setUserOptions((prev) => mergeUserOptions(prev, [], []));
  };

  // Statistics functions
  const getLogSelfStat = async () => {
    const {
      token_name,
      model_name,
      start_timestamp,
      end_timestamp,
      group,
      logType: formLogType,
    } = getFormValues();
    const currentLogType = formLogType !== undefined ? formLogType : logType;
    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    let url = `/api/log/self/stat?type=${currentLogType}&token_name=${token_name}&model_name=${model_name}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&group=${group}`;
    url = encodeURI(url);
    let res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      setStat(data);
    } else {
      showError(message);
    }
  };

  const getLogStat = async () => {
    const {
      usernames,
      token_name,
      model_name,
      start_timestamp,
      end_timestamp,
      channel,
      group,
      logType: formLogType,
    } = getFormValues();
    const currentLogType = formLogType !== undefined ? formLogType : logType;
    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    const params = new URLSearchParams({
      type: `${currentLogType}`,
      token_name,
      model_name,
      start_timestamp: `${localStartTimestamp}`,
      end_timestamp: `${localEndTimestamp}`,
      channel: `${channel || ''}`,
      group,
    });
    usernames.forEach((username) => params.append('usernames', username));
    let res = await API.get(`/api/log/stat?${params.toString()}`);
    const { success, message, data } = res.data;
    if (success) {
      setStat(data);
    } else {
      showError(message);
    }
  };

  const handleEyeClick = async () => {
    if (loadingStat) {
      return;
    }
    setLoadingStat(true);
    if (isAdminUser) {
      await getLogStat();
    } else {
      await getLogSelfStat();
    }
    setShowStat(true);
    setLoadingStat(false);
  };

  // User info function
  const showUserInfoFunc = async (userId) => {
    if (!isAdminUser) {
      return;
    }
    const res = await API.get(`/api/user/${userId}`);
    const { success, message, data } = res.data;
    if (success) {
      setUserInfoData(data);
      setShowUserInfoModal(true);
    } else {
      showError(message);
    }
  };

  const openChannelAffinityUsageCacheModal = (affinity) => {
    const a = affinity || {};
    setChannelAffinityUsageCacheTarget({
      rule_name: a.rule_name || a.reason || '',
      using_group: a.using_group || '',
      key_hint: a.key_hint || '',
      key_fp: a.key_fp || '',
    });
    setShowChannelAffinityUsageCacheModal(true);
  };

  // Format logs data
  const setLogsFormat = (logs) => {
    const requestConversionDisplayValue = (conversionChain) => {
      const chain = Array.isArray(conversionChain)
        ? conversionChain.filter(Boolean)
        : [];
      if (chain.length <= 1) {
        return t('原生格式');
      }
      return `${chain.join(' -> ')}`;
    };

    let expandDatesLocal = {};
    for (let i = 0; i < logs.length; i++) {
      logs[i].timestamp2string = timestamp2string(logs[i].created_at);
      logs[i].key = logs[i].id;
      let other = getLogOther(logs[i].other);
      let expandDataLocal = [];

      if (
        isAdminUser &&
        (logs[i].type === 0 || logs[i].type === 2 || logs[i].type === 6)
      ) {
        expandDataLocal.push({
          key: t('渠道信息'),
          value: `${logs[i].channel} - ${logs[i].channel_name || '[未知]'}`,
        });
      }
      if (logs[i].request_id) {
        expandDataLocal.push({
          key: t('Request ID'),
          value: logs[i].request_id,
        });
      }
      if (isAdminUser && other?.generated_seconds > 0) {
        expandDataLocal.push({
          key: t('发起请求秒数'),
          value: `${formatLogSeconds(other.generated_seconds)} ${t('秒')}`,
        });
      }
      if (isAdminUser && other?.reference_video_seconds_total > 0) {
        expandDataLocal.push({
          key: t('参考视频秒数'),
          value: `${formatLogSeconds(other.reference_video_seconds_total)} ${t('秒')}`,
        });
      }
      if (
        isAdminUser &&
        other?.billing_seconds_total > 0 &&
        (logs[i].type === 6 || other?.reference_video_seconds_total > 0)
      ) {
        expandDataLocal.push({
          key: t('计费总秒数'),
          value: `${formatLogSeconds(other.billing_seconds_total)} ${t('秒')}`,
        });
      }
      if (other?.ws || other?.audio) {
        expandDataLocal.push({
          key: t('语音输入'),
          value: other.audio_input,
        });
        expandDataLocal.push({
          key: t('语音输出'),
          value: other.audio_output,
        });
        expandDataLocal.push({
          key: t('文字输入'),
          value: other.text_input,
        });
        expandDataLocal.push({
          key: t('文字输出'),
          value: other.text_output,
        });
      }
      if (other?.cache_tokens > 0) {
        expandDataLocal.push({
          key: t('缓存 Tokens'),
          value: other.cache_tokens,
        });
      }
      if (other?.cache_creation_tokens > 0) {
        expandDataLocal.push({
          key: t('缓存创建 Tokens'),
          value: other.cache_creation_tokens,
        });
      }
      if (logs[i].type === 2) {
        expandDataLocal.push({
          key: t('日志详情'),
          value: other?.claude
            ? renderClaudeLogContent(
                other?.model_ratio,
                other.completion_ratio,
                other.model_price,
                other.group_ratio,
                other?.user_group_ratio,
                other.cache_ratio || 1.0,
                other.cache_creation_ratio || 1.0,
                other.cache_creation_tokens_5m || 0,
                other.cache_creation_ratio_5m ||
                  other.cache_creation_ratio ||
                  1.0,
                other.cache_creation_tokens_1h || 0,
                other.cache_creation_ratio_1h ||
                  other.cache_creation_ratio ||
                  1.0,
              )
            : renderLogContent(
                other?.model_ratio,
                other.completion_ratio,
                other.model_price,
                other.group_ratio,
                other?.user_group_ratio,
                other.cache_ratio || 1.0,
                false,
                1.0,
                other.web_search || false,
                other.web_search_call_count || 0,
                other.file_search || false,
                other.file_search_call_count || 0,
              ),
        });
        if (logs[i]?.content) {
          expandDataLocal.push({
            key: t('其他详情'),
            value: logs[i].content,
          });
        }
        if (isAdminUser && other?.reject_reason) {
          expandDataLocal.push({
            key: t('拦截原因'),
            value: other.reject_reason,
          });
        }
      }
      if (logs[i].type === 2) {
        let modelMapped =
          other?.is_model_mapped &&
          other?.upstream_model_name &&
          other?.upstream_model_name !== '';
        if (isAdminUser && modelMapped) {
          expandDataLocal.push({
            key: t('请求并计费模型'),
            value: logs[i].model_name,
          });
          expandDataLocal.push({
            key: t('实际模型'),
            value: other.upstream_model_name,
          });
        }

        const isViolationFeeLog =
          other?.violation_fee === true ||
          Boolean(other?.violation_fee_code) ||
          Boolean(other?.violation_fee_marker);

        let content = '';
        if (!isViolationFeeLog) {
          if (other?.ws || other?.audio) {
            content = renderAudioModelPrice(
              other?.text_input,
              other?.text_output,
              other?.model_ratio,
              other?.model_price,
              other?.completion_ratio,
              other?.audio_input,
              other?.audio_output,
              other?.audio_ratio,
              other?.audio_completion_ratio,
              other?.group_ratio,
              other?.user_group_ratio,
              other?.cache_tokens || 0,
              other?.cache_ratio || 1.0,
            );
          } else if (other?.claude) {
            content = renderClaudeModelPrice(
              logs[i].prompt_tokens,
              logs[i].completion_tokens,
              other.model_ratio,
              other.model_price,
              other.completion_ratio,
              other.group_ratio,
              other?.user_group_ratio,
              other.cache_tokens || 0,
              other.cache_ratio || 1.0,
              other.cache_creation_tokens || 0,
              other.cache_creation_ratio || 1.0,
              other.cache_creation_tokens_5m || 0,
              other.cache_creation_ratio_5m ||
                other.cache_creation_ratio ||
                1.0,
              other.cache_creation_tokens_1h || 0,
              other.cache_creation_ratio_1h ||
                other.cache_creation_ratio ||
                1.0,
            );
          } else {
            content = renderModelPrice(
              logs[i].prompt_tokens,
              logs[i].completion_tokens,
              other?.model_ratio,
              other?.model_price,
              other?.completion_ratio,
              other?.group_ratio,
              other?.user_group_ratio,
              other?.cache_tokens || 0,
              other?.cache_ratio || 1.0,
              other?.image || false,
              other?.image_ratio || 0,
              other?.image_output || 0,
              other?.web_search || false,
              other?.web_search_call_count || 0,
              other?.web_search_price || 0,
              other?.file_search || false,
              other?.file_search_call_count || 0,
              other?.file_search_price || 0,
              other?.audio_input_seperate_price || false,
              other?.audio_input_token_count || 0,
              other?.audio_input_price || 0,
              other?.image_generation_call || false,
              other?.image_generation_call_price || 0,
            );
          }
          expandDataLocal.push({
            key: t('计费过程'),
            value: content,
          });
        }
        if (other?.reasoning_effort) {
          expandDataLocal.push({
            key: t('Reasoning Effort'),
            value: other.reasoning_effort,
          });
        }
      }
      if (logs[i].type === 6) {
        if (other?.task_id) {
          expandDataLocal.push({
            key: t('任务ID'),
            value: other.task_id,
          });
        }
        if (other?.reason) {
          expandDataLocal.push({
            key: t('失败原因'),
            value: (
              <div
                style={{
                  maxWidth: 600,
                  whiteSpace: 'normal',
                  wordBreak: 'break-word',
                  lineHeight: 1.6,
                }}
              >
                {other.reason}
              </div>
            ),
          });
        }
      }
      if (other?.request_path) {
        expandDataLocal.push({
          key: t('请求路径'),
          value: other.request_path,
        });
      }
      if (other?.billing_source === 'subscription') {
        const planId = other?.subscription_plan_id;
        const planTitle = other?.subscription_plan_title || '';
        const subscriptionId = other?.subscription_id;
        const unit = t('额度');
        const pre = other?.subscription_pre_consumed ?? 0;
        const postDelta = other?.subscription_post_delta ?? 0;
        const finalConsumed = other?.subscription_consumed ?? pre + postDelta;
        const remain = other?.subscription_remain;
        const total = other?.subscription_total;
        // Use multiple Description items to avoid an overlong single line.
        if (planId) {
          expandDataLocal.push({
            key: t('订阅套餐'),
            value: `#${planId} ${planTitle}`.trim(),
          });
        }
        if (subscriptionId) {
          expandDataLocal.push({
            key: t('订阅实例'),
            value: `#${subscriptionId}`,
          });
        }
        const settlementLines = [
          `${t('预扣')}：${pre} ${unit}`,
          `${t('结算差额')}：${postDelta > 0 ? '+' : ''}${postDelta} ${unit}`,
          `${t('最终抵扣')}：${finalConsumed} ${unit}`,
        ]
          .filter(Boolean)
          .join('\n');
        expandDataLocal.push({
          key: t('订阅结算'),
          value: (
            <div style={{ whiteSpace: 'pre-line' }}>{settlementLines}</div>
          ),
        });
        if (remain !== undefined && total !== undefined) {
          expandDataLocal.push({
            key: t('订阅剩余'),
            value: `${remain}/${total} ${unit}`,
          });
        }
        expandDataLocal.push({
          key: t('订阅说明'),
          value: t(
            'token 会按倍率换算成“额度/次数”，请求结束后再做差额结算（补扣/返还）。',
          ),
        });
      }
      if (isAdminUser && logs[i].type !== 6) {
        expandDataLocal.push({
          key: t('请求转换'),
          value: requestConversionDisplayValue(other?.request_conversion),
        });
      }
      if (isAdminUser && logs[i].type !== 6) {
        let localCountMode = '';
        if (other?.admin_info?.local_count_tokens) {
          localCountMode = t('本地计费');
        } else {
          localCountMode = t('上游返回');
        }
        expandDataLocal.push({
          key: t('计费模式'),
          value: localCountMode,
        });
      }
      expandDatesLocal[logs[i].key] = expandDataLocal;
    }

    setExpandData(expandDatesLocal);
    setLogs(logs);
  };

  // Load logs function
  const loadLogs = async (startIdx, pageSize, customLogType = null) => {
    setLoading(true);

    const {
      usernames,
      token_name,
      model_name,
      start_timestamp,
      end_timestamp,
      channel,
      group,
      request_id,
      logType: formLogType,
    } = getFormValues();

    const currentLogType =
      customLogType !== null
        ? customLogType
        : formLogType !== undefined
          ? formLogType
          : logType;

    let localStartTimestamp = Date.parse(start_timestamp) / 1000;
    let localEndTimestamp = Date.parse(end_timestamp) / 1000;
    let url = '';
    if (isAdminUser) {
      const params = new URLSearchParams({
        p: `${startIdx}`,
        page_size: `${pageSize}`,
        type: `${currentLogType}`,
        token_name,
        model_name,
        start_timestamp: `${localStartTimestamp}`,
        end_timestamp: `${localEndTimestamp}`,
        channel: `${channel || ''}`,
        group,
        request_id,
      });
      usernames.forEach((username) => params.append('usernames', username));
      url = `/api/log/?${params.toString()}`;
    } else {
      const params = new URLSearchParams({
        p: `${startIdx}`,
        page_size: `${pageSize}`,
        type: `${currentLogType}`,
        token_name,
        model_name,
        start_timestamp: `${localStartTimestamp}`,
        end_timestamp: `${localEndTimestamp}`,
        group,
        request_id,
      });
      url = `/api/log/self/?${params.toString()}`;
    }
    const res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      const newPageData = data.items;
      setActivePage(data.page);
      setPageSize(data.page_size);
      setLogCount(data.total);

      setLogsFormat(newPageData);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  // Page handlers
  const handlePageChange = (page) => {
    setActivePage(page);
    loadLogs(page, pageSize).then((r) => {});
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    loadLogs(activePage, size)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  };

  // Refresh function
  const refresh = async () => {
    setActivePage(1);
    handleEyeClick();
    await loadLogs(1, pageSize);
  };

  const resetFilters = () => {
    if (!formApi) {
      return;
    }

    if (isAdminUser) {
      setPersistentUsernames([]);
      localStorage.removeItem(ADMIN_USER_FILTER_STORAGE_KEY);
      setUserOptions((prev) => mergeUserOptions(prev, [], []));
    }

    formApi.setValues(getDefaultFormInitValues([]));
    setLogType(0);
    setActivePage(1);
    setTimeout(() => {
      refresh();
    }, 0);
  };

  // Copy text function
  const copyText = async (e, text) => {
    e.stopPropagation();
    if (await copy(text)) {
      showSuccess('已复制：' + text);
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  // Initialize data
  useEffect(() => {
    const localPageSize =
      parseInt(localStorage.getItem('page-size')) || ITEMS_PER_PAGE;
    setPageSize(localPageSize);
    loadLogs(activePage, localPageSize)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  }, []);

  // Initialize statistics when formApi is available
  useEffect(() => {
    if (formApi) {
      handleEyeClick();
    }
  }, [formApi]);

  useEffect(() => {
    return () => {
      if (userSearchTimeoutRef.current) {
        clearTimeout(userSearchTimeoutRef.current);
      }
    };
  }, []);

  // Check if any record has expandable content
  const hasExpandableRows = () => {
    return logs.some(
      (log) => expandData[log.key] && expandData[log.key].length > 0,
    );
  };

  return {
    // Basic state
    logs,
    expandData,
    showStat,
    loading,
    loadingStat,
    activePage,
    logCount,
    pageSize,
    logType,
    stat,
    isAdminUser,

    // Form state
    formApi,
    setFormApi,
    formInitValues,
    getFormValues,

    // Column visibility
    visibleColumns,
    showColumnSelector,
    setShowColumnSelector,
    handleColumnVisibilityChange,
    handleSelectAll,
    initDefaultColumns,
    COLUMN_KEYS,

    // Compact mode
    compactMode,
    setCompactMode,

    // User info modal
    showUserInfo,
    setShowUserInfoModal,
    userInfoData,
    showUserInfoFunc,
    userOptions,
    userOptionsLoading,
    handleUserSearch,
    handleUserSelectionChange,
    handleUserDropdownVisibleChange,
    clearPersistentUsernames,
    persistentUsernames,

    // Channel affinity usage cache stats modal
    showChannelAffinityUsageCacheModal,
    setShowChannelAffinityUsageCacheModal,
    channelAffinityUsageCacheTarget,
    openChannelAffinityUsageCacheModal,

    // Functions
    loadLogs,
    handlePageChange,
    handlePageSizeChange,
    refresh,
    resetFilters,
    copyText,
    handleEyeClick,
    setLogsFormat,
    hasExpandableRows,
    setLogType,

    // Translation
    t,
  };
};
