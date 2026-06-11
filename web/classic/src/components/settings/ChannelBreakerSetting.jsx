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

import React, { useEffect, useState } from 'react';
import { Card, Spin } from '@douyinfe/semi-ui';
import SettingsChannelBreaker from '../../pages/Setting/ChannelBreaker/SettingsChannelBreaker';
import { API, showError, toBoolean } from '../../helpers';

const ChannelBreakerSetting = () => {
  const [inputs, setInputs] = useState({
    ChannelBreakerEnabled: false,
    ChannelBreakerFailureLimit: '5',
    ChannelBreakerCooldownSeconds: '60',
    ChannelBreakerProbeCount: '5',
    ChannelBreakerProbeSuccessCount: '3',
    ChannelBreakerExcludePaths: '/v1/videos',
    ChannelBreakerRules: '[]',
    ChannelBreakerExemptChannels: '[]',
    AutomaticDisableKeywords: '',
    ChannelBreakerFailureStatusCodes:
      '100-199,300-399,401-407,409-499,500-503,505-523,525-599',
    'monitor_setting.bark_alert_enabled': true,
    'monitor_setting.bark_alert_url':
      'https://bark.aigod.one/kFRNZMUXcuQ6c4ccrUgQ3W/',
    'monitor_setting.low_balance_alert_enabled': true,
    'monitor_setting.low_balance_threshold_cny': 10,
    'monitor_setting.channel_breaker_alert_enabled': true,
  });
  const [loading, setLoading] = useState(false);

  const getOptions = async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (success) {
      const newInputs = {};
      data.forEach((item) => {
        if (typeof inputs[item.key] === 'boolean') {
          newInputs[item.key] = toBoolean(item.value);
        } else {
          newInputs[item.key] = item.value;
        }
      });
      setInputs(newInputs);
    } else {
      showError(message);
    }
  };

  const onRefresh = async () => {
    try {
      setLoading(true);
      await getOptions();
    } catch (error) {
      showError('刷新失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    onRefresh();
  }, []);

  return (
    <Spin spinning={loading} size='large'>
      <Card style={{ marginTop: '10px' }}>
        <SettingsChannelBreaker options={inputs} refresh={onRefresh} />
      </Card>
    </Spin>
  );
};

export default ChannelBreakerSetting;
