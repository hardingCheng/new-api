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
import { Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  Moonshot,
  OpenAI,
  XAI,
  Zhipu,
  Volcengine,
  Cohere,
  Claude,
  Gemini,
  Suno,
  Minimax,
  Wenxin,
  Spark,
  Qingyan,
  DeepSeek,
  Qwen,
  Midjourney,
  Grok,
  AzureAI,
  Hunyuan,
  Xinference,
} from '@lobehub/icons';

const { Text } = Typography;

const ProviderIcons = () => {
  const { t } = useTranslation();

  const providers = [
    { Icon: Moonshot, size: 40 },
    { Icon: OpenAI, size: 40 },
    { Icon: XAI, size: 40 },
    { Icon: Zhipu.Color, size: 40 },
    { Icon: Volcengine.Color, size: 40 },
    { Icon: Cohere.Color, size: 40 },
    { Icon: Claude.Color, size: 40 },
    { Icon: Gemini.Color, size: 40 },
    { Icon: Suno, size: 40 },
    { Icon: Minimax.Color, size: 40 },
    { Icon: Wenxin.Color, size: 40 },
    { Icon: Spark.Color, size: 40 },
    { Icon: Qingyan.Color, size: 40 },
    { Icon: DeepSeek.Color, size: 40 },
    { Icon: Qwen.Color, size: 40 },
    { Icon: Midjourney, size: 40 },
    { Icon: Grok, size: 40 },
    { Icon: AzureAI.Color, size: 40 },
    { Icon: Hunyuan.Color, size: 40 },
    { Icon: Xinference.Color, size: 40 },
  ];

  return (
    <div className='mt-12 md:mt-16 lg:mt-20 w-full'>
      <div className='flex items-center mb-6 md:mb-8 justify-center'>
        <Text
          type='tertiary'
          className='text-lg md:text-xl lg:text-2xl font-light'
        >
          {t('支持众多的大模型供应商')}
        </Text>
      </div>
      <div className='flex flex-wrap items-center justify-center gap-3 sm:gap-4 md:gap-6 lg:gap-8 max-w-5xl mx-auto px-4'>
        {providers.map((item, index) => (
          <div
            key={index}
            className='w-8 h-8 sm:w-10 sm:h-10 md:w-12 md:h-12 flex items-center justify-center icon-float hover-scale'
            style={{ animationDelay: `${index * 0.1}s` }}
          >
            <item.Icon size={item.size} />
          </div>
        ))}
        <div className='w-8 h-8 sm:w-10 sm:h-10 md:w-12 md:h-12 flex items-center justify-center icon-float hover-scale'>
          <Typography.Text className='!text-lg sm:!text-xl md:!text-2xl lg:!text-3xl font-bold gradient-text'>
            30+
          </Typography.Text>
        </div>
      </div>
    </div>
  );
};

export default ProviderIcons;
