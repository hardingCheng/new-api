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

// 图像生成模型识别关键词
export const IMAGE_MODEL_KEYWORD = '-image';

// 默认图像生成模型列表（当无法从API获取时使用）
// 注意：使用前请确认您的令牌是否已启用所选模型，未启用的模型将无法使用
export const DEFAULT_IMAGE_MODELS = [
  'gemini-3.1-flash-image-preview',
  'gemini-3-pro-image-preview',
  'gemini-2.5-flash-image-preview',
  'gemini-2.5-flash-image',
];

// 分辨率选项
export const RESOLUTION_OPTIONS = [
  {
    key: '1k',
    label: '1K',
    baseSize: 1024,
    description: '标准质量，适合日常使用',
  },
  {
    key: '2k',
    label: '2K',
    baseSize: 2048,
    description: '高质量，适合打印输出',
  },
  {
    key: '4k',
    label: '4K',
    baseSize: 4096,
    description: '超高质量，适合专业用途',
  },
];

// 比例选项
export const ASPECT_RATIO_OPTIONS = [
  {
    key: '1:1',
    label: '1:1',
    name: '正方形',
    width: 1,
    height: 1,
    icon: 'Square',
    description: '头像、图标、社交媒体',
  },
  {
    key: '16:9',
    label: '16:9',
    name: '横屏宽幅',
    width: 16,
    height: 9,
    icon: 'RectangleHorizontal',
    description: '视频封面、桌面壁纸',
  },
  {
    key: '9:16',
    label: '9:16',
    name: '竖屏',
    width: 9,
    height: 16,
    icon: 'RectangleVertical',
    description: '手机壁纸、短视频封面',
  },
  {
    key: '4:3',
    label: '4:3',
    name: '横屏标准',
    width: 4,
    height: 3,
    icon: 'RectangleHorizontal',
    description: '传统显示器、PPT',
  },
  {
    key: '3:4',
    label: '3:4',
    name: '竖屏标准',
    width: 3,
    height: 4,
    icon: 'RectangleVertical',
    description: '竖版海报、产品图',
  },
  {
    key: '3:2',
    label: '3:2',
    name: '横屏照片',
    width: 3,
    height: 2,
    icon: 'RectangleHorizontal',
    description: '相机照片比例',
  },
  {
    key: '2:3',
    label: '2:3',
    name: '竖屏照片',
    width: 2,
    height: 3,
    icon: 'RectangleVertical',
    description: '竖版照片、人像',
  },
  {
    key: '21:9',
    label: '21:9',
    name: '超宽屏',
    width: 21,
    height: 9,
    icon: 'RectangleHorizontal',
    description: '电影画幅、带鱼屏',
  },
];

// 生成状态
export const GENERATION_STATUS = {
  IDLE: 'idle',
  LOADING: 'loading',
  SUCCESS: 'success',
  ERROR: 'error',
};

// 本地存储键
export const STORAGE_KEYS = {
  HISTORY: 'banana_image_history',
  SETTINGS: 'banana_image_settings',
};

// 历史记录最大数量
export const MAX_HISTORY_RECORDS = 100;

// API 端点
export const API_ENDPOINTS = {
  MODELS: '/api/user/models',
  IMAGE_GENERATIONS: '/v1/images/generations',
  CHAT_COMPLETIONS: '/v1/chat/completions',
};

/**
 * 根据分辨率和比例计算实际图像尺寸
 * @param {string} resolution - 分辨率档位 ('1k' | '2k' | '4k')
 * @param {string} aspectRatio - 图像比例 ('1:1' | '16:9' | ...)
 * @returns {{ width: number, height: number }}
 */
export const calculateImageSize = (resolution, aspectRatio) => {
  const resolutionConfig = RESOLUTION_OPTIONS.find((r) => r.key === resolution);
  const ratioConfig = ASPECT_RATIO_OPTIONS.find((r) => r.key === aspectRatio);

  if (!resolutionConfig || !ratioConfig) {
    return { width: 1024, height: 1024 };
  }

  const baseSize = resolutionConfig.baseSize;
  const { width: ratioW, height: ratioH } = ratioConfig;

  // 以短边为基准计算，确保短边等于 baseSize
  if (ratioW >= ratioH) {
    // 横向或正方形：高度为基准
    const height = baseSize;
    const width = Math.round((baseSize * ratioW) / ratioH);
    return { width, height };
  } else {
    // 纵向：宽度为基准
    const width = baseSize;
    const height = Math.round((baseSize * ratioH) / ratioW);
    return { width, height };
  }
};

/**
 * 从模型列表中过滤图像生成模型
 * @param {Array} models - 模型列表
 * @returns {Array} 过滤后的图像生成模型列表
 */
export const filterImageModels = (models) => {
  const filtered = models.filter((model) => {
    const modelId = typeof model === 'string' ? model : model.id || model.value;
    return modelId.toLowerCase().includes(IMAGE_MODEL_KEYWORD.toLowerCase());
  });

  // 确保 DEFAULT_IMAGE_MODELS 中的模型都包含在结果中
  const filteredIds = new Set(
    filtered.map((model) => (typeof model === 'string' ? model : model.id || model.value))
  );

  DEFAULT_IMAGE_MODELS.forEach((defaultModel) => {
    if (!filteredIds.has(defaultModel)) {
      // 如果默认模型不在过滤结果中，添加它
      filtered.push(
        typeof filtered[0] === 'string'
          ? defaultModel
          : { id: defaultModel, value: defaultModel, label: defaultModel }
      );
    }
  });

  return filtered;
};
