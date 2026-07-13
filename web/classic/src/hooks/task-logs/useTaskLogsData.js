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

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@douyinfe/semi-ui';
import {
  API,
  copy,
  isAdmin,
  showError,
  showSuccess,
  showWarning,
  timestamp2string,
} from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';
import { buildTaskExportRows } from '../../components/table/task-logs/taskLogsExport';

export const useTaskLogsData = () => {
  const { t } = useTranslation();

  // Define column keys for selection
  const COLUMN_KEYS = {
    SUBMIT_TIME: 'submit_time',
    FINISH_TIME: 'finish_time',
    DURATION: 'duration',
    CHANNEL_NAME: 'channel_name',
    CHANNEL_ID: 'channel_id',
    USERNAME: 'username',
    PLATFORM: 'platform',
    MODEL: 'model',
    QUOTA: 'quota',
    REFUND_QUOTA: 'refund_quota',
    VIDEO_DURATION: 'video_duration',
    HAS_REFERENCE_VIDEO: 'has_reference_video',
    REFERENCE_VIDEO_DURATION: 'reference_video_duration',
    TYPE: 'type',
    TASK_ID: 'task_id',
    TASK_STATUS: 'task_status',
    PROGRESS: 'progress',
    FAIL_REASON: 'fail_reason',
    RESULT_URL: 'result_url',
  };

  // Columns visible to admins only
  const ADMIN_ONLY_COLUMNS = [
    COLUMN_KEYS.CHANNEL_NAME,
    COLUMN_KEYS.CHANNEL_ID,
    COLUMN_KEYS.USERNAME,
    COLUMN_KEYS.QUOTA,
    COLUMN_KEYS.REFUND_QUOTA,
    COLUMN_KEYS.HAS_REFERENCE_VIDEO,
    COLUMN_KEYS.REFERENCE_VIDEO_DURATION,
  ];

  // Basic state
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [quotaPools, setQuotaPools] = useState([]);
  const [activePage, setActivePage] = useState(1);
  const [logCount, setLogCount] = useState(0);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);

  // User and admin
  const isAdminUser = isAdmin();
  // Role-specific storage key to prevent different roles from overwriting each other
  const STORAGE_KEY = isAdminUser
    ? 'task-logs-table-columns-admin'
    : 'task-logs-table-columns-user';

  // Modal state
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [modalContent, setModalContent] = useState('');

  // 新增：视频预览弹窗状态
  const [isVideoModalOpen, setIsVideoModalOpen] = useState(false);
  const [videoUrl, setVideoUrl] = useState('');

  // Audio preview modal state
  const [isAudioModalOpen, setIsAudioModalOpen] = useState(false);
  const [audioClips, setAudioClips] = useState([]);

  // User info modal state
  const [showUserInfo, setShowUserInfoModal] = useState(false);
  const [userInfoData, setUserInfoData] = useState(null);

  // Form state
  const [formApi, setFormApi] = useState(null);
  let now = new Date();
  let zeroNow = new Date(now.getFullYear(), now.getMonth(), now.getDate());

  const formInitValues = {
    channel_id: '',
    task_id: '',
    status: '',
    username: '',
    model_name: '',
    dateRange: [
      timestamp2string(zeroNow.getTime() / 1000),
      timestamp2string(now.getTime() / 1000 + 3600),
    ],
  };

  // Column visibility state
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);

  // Compact mode
  const [compactMode, setCompactMode] = useTableCompactMode('taskLogs');

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
          ADMIN_ONLY_COLUMNS.forEach((key) => {
            merged[key] = false;
          });
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

  // Get default column visibility based on user role
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.SUBMIT_TIME]: true,
      [COLUMN_KEYS.FINISH_TIME]: true,
      [COLUMN_KEYS.DURATION]: true,
      [COLUMN_KEYS.CHANNEL_NAME]: isAdminUser,
      [COLUMN_KEYS.CHANNEL_ID]: isAdminUser,
      [COLUMN_KEYS.USERNAME]: isAdminUser,
      [COLUMN_KEYS.PLATFORM]: true,
      [COLUMN_KEYS.MODEL]: true,
      [COLUMN_KEYS.QUOTA]: isAdminUser,
      [COLUMN_KEYS.REFUND_QUOTA]: isAdminUser,
      [COLUMN_KEYS.VIDEO_DURATION]: true,
      [COLUMN_KEYS.HAS_REFERENCE_VIDEO]: isAdminUser,
      [COLUMN_KEYS.REFERENCE_VIDEO_DURATION]: isAdminUser,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.TASK_ID]: true,
      [COLUMN_KEYS.TASK_STATUS]: true,
      [COLUMN_KEYS.PROGRESS]: true,
      [COLUMN_KEYS.FAIL_REASON]: true,
      [COLUMN_KEYS.RESULT_URL]: true,
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
      if (!isAdminUser && ADMIN_ONLY_COLUMNS.includes(key)) {
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

  // Get form values helper function
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};

    // 处理时间范围
    let start_timestamp = timestamp2string(zeroNow.getTime() / 1000);
    let end_timestamp = timestamp2string(now.getTime() / 1000 + 3600);

    if (
      formValues.dateRange &&
      Array.isArray(formValues.dateRange) &&
      formValues.dateRange.length === 2
    ) {
      start_timestamp = formValues.dateRange[0];
      end_timestamp = formValues.dateRange[1];
    }

    return {
      channel_id: formValues.channel_id || '',
      task_id: formValues.task_id || '',
      status: formValues.status || '',
      username: formValues.username || '',
      model_name: formValues.model_name || '',
      start_timestamp,
      end_timestamp,
    };
  };

  // Enrich logs data
  const enrichLogs = (items) => {
    return items.map((log) => ({
      ...log,
      timestamp2string: timestamp2string(log.created_at),
      key: '' + log.id,
    }));
  };

  // Sync page data
  const syncPageData = (payload) => {
    const items = enrichLogs(payload.items || []);
    setLogs(items);
    setLogCount(payload.total || 0);
    setActivePage(payload.page || 1);
    setPageSize(payload.page_size || pageSize);
  };

  // Load logs function
  const loadLogs = async (page = 1, size = pageSize) => {
    setLoading(true);
    const {
      channel_id,
      task_id,
      status,
      username,
      model_name,
      start_timestamp,
      end_timestamp,
    } = getFormValues();
    let localStartTimestamp = parseInt(Date.parse(start_timestamp) / 1000);
    let localEndTimestamp = parseInt(Date.parse(end_timestamp) / 1000);
    const params = new URLSearchParams({
      p: String(page),
      page_size: String(size),
      task_id,
      start_timestamp: String(localStartTimestamp),
      end_timestamp: String(localEndTimestamp),
    });
    if (isAdminUser) {
      params.set('channel_id', channel_id);
      params.set('status', status);
      params.set('username', username);
      params.set('model_name', model_name);
    }
    const url = isAdminUser
      ? `/api/task/?${params.toString()}`
      : `/api/task/self?${params.toString()}`;
    const res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      syncPageData(data);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const loadQuotaPools = async () => {
    try {
      const res = await API.get('/api/task/model_quota_pools');
      const { success, data } = res.data;
      if (success) {
        setQuotaPools(Array.isArray(data) ? data : []);
      } else {
        setQuotaPools([]);
        console.warn('Failed to load model quota pools:', res.data?.message);
      }
    } catch (error) {
      setQuotaPools([]);
      console.warn('Failed to load model quota pools:', error);
    }
  };

  // Page handlers
  const handlePageChange = (page) => {
    loadLogs(page, pageSize).then();
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('task-page-size', size + '');
    await loadLogs(1, size);
  };

  // Refresh function
  const refresh = async () => {
    await Promise.all([loadLogs(1, pageSize), loadQuotaPools()]);
  };

  // 导出报表：按当前筛选条件翻页拉取全部记录，生成 xlsx 下载
  const exportReport = async () => {
    setExporting(true);
    try {
      const {
        channel_id,
        task_id,
        status,
        username,
        model_name,
        start_timestamp,
        end_timestamp,
      } = getFormValues();
      const localStartTimestamp = parseInt(Date.parse(start_timestamp) / 1000);
      const localEndTimestamp = parseInt(Date.parse(end_timestamp) / 1000);

      // 后端导出接口要求有限时间范围，并对返回行数设置硬上限。
      const params = new URLSearchParams({
        task_id,
        start_timestamp: String(localStartTimestamp),
        end_timestamp: String(localEndTimestamp),
      });
      if (isAdminUser) {
        params.set('channel_id', channel_id);
        params.set('status', status);
        params.set('username', username);
        params.set('model_name', model_name);
      }
      // 导出按钮仅管理员可见，固定走管理员导出接口。
      const url = `/api/task/export?${params.toString()}`;
      const res = await API.get(url);
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      const allItems = data.items || [];

      if (allItems.length === 0) {
        showWarning(t('没有可导出的数据'));
        return;
      }

      const rows = buildTaskExportRows(allItems, { t, isAdminUser });
      const XLSX = await import('xlsx');
      const worksheet = XLSX.utils.json_to_sheet(rows);
      const workbook = XLSX.utils.book_new();
      XLSX.utils.book_append_sheet(workbook, worksheet, t('任务记录'));
      const stamp = timestamp2string(Date.now() / 1000).replace(/[^\d]/g, '');
      XLSX.writeFile(workbook, `task-report-${stamp}.xlsx`);
      showSuccess(t('导出成功'));
    } catch (error) {
      console.error('export task report failed', error);
      showError(t('导出失败，请重试'));
    } finally {
      setExporting(false);
    }
  };

  // Copy text function
  const copyText = async (text) => {
    if (await copy(text)) {
      showSuccess(t('已复制：') + text);
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  // Modal handlers
  const openContentModal = (content) => {
    setModalContent(content);
    setIsModalOpen(true);
  };

  // 新增：打开视频预览弹窗
  const openVideoModal = (url) => {
    setVideoUrl(url);
    setIsVideoModalOpen(true);
  };

  const openAudioModal = (clips) => {
    setAudioClips(clips);
    setIsAudioModalOpen(true);
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

  // Initialize data
  useEffect(() => {
    const localPageSize =
      parseInt(localStorage.getItem('task-page-size')) || ITEMS_PER_PAGE;
    setPageSize(localPageSize);
    loadLogs(1, localPageSize).then();
    loadQuotaPools().then();
  }, []);

  return {
    // Basic state
    logs,
    loading,
    exporting,
    quotaPools,
    activePage,
    logCount,
    pageSize,
    isAdminUser,

    // Modal state
    isModalOpen,
    setIsModalOpen,
    modalContent,

    // 新增：视频弹窗状态
    isVideoModalOpen,
    setIsVideoModalOpen,
    videoUrl,

    // Audio preview modal
    isAudioModalOpen,
    setIsAudioModalOpen,
    audioClips,

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
    ADMIN_ONLY_COLUMNS,

    // Compact mode
    compactMode,
    setCompactMode,

    // User info modal
    showUserInfo,
    setShowUserInfoModal,
    userInfoData,
    showUserInfoFunc,

    // Functions
    loadLogs,
    handlePageChange,
    handlePageSizeChange,
    refresh,
    exportReport,
    copyText,
    openContentModal,
    openVideoModal,
    openAudioModal,
    enrichLogs,
    syncPageData,

    // Translation
    t,
  };
};
