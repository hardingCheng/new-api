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

import { useState, useCallback, useEffect } from 'react';
import { Toast } from '@douyinfe/semi-ui';
import { API } from '../../helpers';
import {
  GENERATION_STATUS,
  STORAGE_KEYS,
  MAX_HISTORY_RECORDS,
  filterImageModels,
  calculateImageSize,
  DEFAULT_IMAGE_MODELS,
} from '../../constants/banana-image.constants';
import {
  saveImageToCache,
  getCacheStats,
  getAllHistoryRecords,
  getHistoryRecordsPaginated,
  getHistoryRecordsCount,
  searchHistoryRecords,
  saveHistoryRecord,
  deleteHistoryRecord as deleteHistoryFromDB,
  clearAllHistory,
} from '../../utils/imageCache';

// 获取服务器地址
const getServerAddress = () => {
  let status = localStorage.getItem('status');
  let serverAddress = '';
  if (status) {
    try {
      status = JSON.parse(status);
      serverAddress = status.server_address || '';
    } catch (error) {
      console.error('Failed to parse status:', error);
    }
  }
  if (!serverAddress) {
    serverAddress = window.location.origin;
  }
  return serverAddress;
};

// 默认状态
const defaultState = {
  // 提示词
  prompt: '',
  negativePrompt: '',

  // 令牌
  selectedToken: null,
  availableTokens: [],
  tokensLoading: false,

  // 模型
  selectedModel: '',
  availableModels: [],
  modelsLoading: false,

  // 参数
  resolution: '1k',
  aspectRatio: '1:1',
  numberOfImages: 1,
  referenceImages: [], // 参考图片

  // 生成状态
  generationStatus: GENERATION_STATUS.IDLE,
  generationError: null,
  generatedImages: [],
  selectedImageIndex: 0,
  generationStartTime: null, // 生成开始时间
  retryMessage: null, // 重试提示信息

  // 历史记录
  historyRecords: [],
  historyPage: 1,
  historyPageSize: 10,
  historyHasMore: true,
  historyTotalCount: 0,
  historyLoading: false,

  // 缓存统计
  cacheStats: {
    count: 0,
    totalSize: 0,
    oldestTimestamp: Date.now(),
  },
};

// 初始状态（从本地存储恢复）
const getInitialState = () => {
  try {
    const savedSettings = localStorage.getItem(STORAGE_KEYS.SETTINGS);
    if (savedSettings) {
      const settings = JSON.parse(savedSettings);
      return {
        ...defaultState,
        resolution: settings.resolution || '1k',
        aspectRatio: settings.aspectRatio || '1:1',
        numberOfImages: settings.numberOfImages || 1,
      };
    }
  } catch (e) {
    console.error('Failed to load saved settings:', e);
  }
  return defaultState;
};

export const useBananaImage = () => {
  const [state, setState] = useState(getInitialState);

  // 计算当前尺寸
  const currentSize = calculateImageSize(state.resolution, state.aspectRatio);

  // 更新单个字段
  const updateField = useCallback((field, value) => {
    setState((prev) => ({ ...prev, [field]: value }));
  }, []);

  // 批量更新字段
  const updateFields = useCallback((fields) => {
    setState((prev) => ({ ...prev, ...fields }));
  }, []);

  // 保存设置到本地存储
  const saveSettings = useCallback(() => {
    try {
      const settingsToSave = {
        resolution: state.resolution,
        aspectRatio: state.aspectRatio,
        numberOfImages: state.numberOfImages,
      };
      localStorage.setItem(STORAGE_KEYS.SETTINGS, JSON.stringify(settingsToSave));
    } catch (e) {
      console.error('Failed to save settings:', e);
    }
  }, [state.resolution, state.aspectRatio, state.numberOfImages]);

  // 设置变化时自动保存
  useEffect(() => {
    saveSettings();
  }, [saveSettings]);

  // 加载令牌列表
  const loadTokens = useCallback(async () => {
    updateField('tokensLoading', true);
    try {
      const res = await API.get('/api/token/?p=1&size=100');
      const { success, data } = res.data;

      if (success) {
        const tokenItems = Array.isArray(data) ? data : data.items || [];
        // 只获取启用状态的令牌
        const activeTokens = tokenItems.filter((token) => token.status === 1);
        const tokenOptions = activeTokens.map((token) => ({
          label: token.name,
          value: token.key,
          id: token.id,
          name: token.name,
          key: token.key,
          group: token.group, // 令牌所属分组
        }));

        updateFields({
          availableTokens: tokenOptions,
          tokensLoading: false,
        });

        // 如果有令牌，默认选择第一个
        if (tokenOptions.length > 0 && !state.selectedToken) {
          handleTokenChange(tokenOptions[0]);
        }
      } else {
        updateField('tokensLoading', false);
      }
    } catch (error) {
      console.error('Failed to load tokens:', error);
      updateField('tokensLoading', false);
    }
  }, [updateField, updateFields]);

  // 根据令牌加载可用模型
  const loadModelsForToken = useCallback(
    async (token) => {
      if (!token) {
        updateFields({
          availableModels: [],
          selectedModel: '',
        });
        return;
      }

      const tokenKey = token.key || token.value;
      const tokenGroup = token.group || '';

      updateField('modelsLoading', true);

      // 辅助函数：设置模型选项
      const setModelOptions = (modelOptions) => {
        // 按照 DEFAULT_IMAGE_MODELS 的顺序对模型进行排序
        const sortedModelOptions = [...modelOptions].sort((a, b) => {
          const indexA = DEFAULT_IMAGE_MODELS.indexOf(a.value);
          const indexB = DEFAULT_IMAGE_MODELS.indexOf(b.value);
          
          // 如果两个模型都在默认列表中，按默认列表顺序排序
          if (indexA !== -1 && indexB !== -1) {
            return indexA - indexB;
          }
          // 如果只有 a 在默认列表中，a 排在前面
          if (indexA !== -1) {
            return -1;
          }
          // 如果只有 b 在默认列表中，b 排在前面
          if (indexB !== -1) {
            return 1;
          }
          // 如果都不在默认列表中，按字母顺序排序
          return a.value.localeCompare(b.value);
        });
        
        updateFields({
          availableModels: sortedModelOptions,
          modelsLoading: false,
          selectedModel:
            sortedModelOptions.some((m) => m.value === state.selectedModel) && state.selectedModel
              ? state.selectedModel
              : sortedModelOptions[0]?.value || '',
        });
      };

      // 辅助函数：使用默认模型
      const useDefaultModels = () => {
        const defaultModelOptions = DEFAULT_IMAGE_MODELS.map((model) => ({
          label: model,
          value: model,
        }));
        setModelOptions(defaultModelOptions);
      };

      try {
        // 使用内部 API 获取模型列表，带上分组参数
        let apiUrl = '/api/user/models';
        if (tokenGroup) {
          apiUrl += `?group=${encodeURIComponent(tokenGroup)}`;
        }

        const res = await API.get(apiUrl);
        const { success, data } = res.data;

        if (success && Array.isArray(data) && data.length > 0) {
          // 过滤出图像生成模型
          const imageModels = filterImageModels(data);

          if (imageModels.length > 0) {
            const modelOptions = imageModels.map((model) => ({
              label: model,
              value: model,
            }));
            setModelOptions(modelOptions);
          } else {
            // 内部 API 没有图像模型，使用默认模型
            useDefaultModels();
          }
        } else {
          // 内部 API 失败或无数据，尝试使用 OpenAI 兼容 API
          const serverAddress = getServerAddress();
          const extRes = await fetch(`${serverAddress}/v1/models`, {
            headers: {
              Authorization: `Bearer sk-${tokenKey}`,
              'Content-Type': 'application/json',
            },
          });

          if (!extRes.ok) {
            throw new Error(`HTTP error! status: ${extRes.status}`);
          }

          const result = await extRes.json();
          const models = result.data || [];

          // 过滤出图像生成模型
          const imageModels = filterImageModels(models.map((m) => m.id));

          if (imageModels.length > 0) {
            const modelOptions = imageModels.map((model) => ({
              label: model,
              value: model,
            }));
            setModelOptions(modelOptions);
          } else {
            // 外部 API 也没有图像模型，使用默认模型
            useDefaultModels();
          }
        }
      } catch (error) {
        console.error('Failed to load models:', error);
        // 加载失败时使用默认模型
        useDefaultModels();
      }
    },
    [updateField, updateFields, state.selectedModel]
  );

  // 处理令牌变更
  const handleTokenChange = useCallback(
    (token) => {
      updateField('selectedToken', token);
      if (token) {
        loadModelsForToken(token);
      } else {
        updateFields({
          availableModels: [],
          selectedModel: '',
        });
      }
    },
    [updateField, updateFields, loadModelsForToken]
  );

  // 初始加载令牌
  useEffect(() => {
    loadTokens();
  }, []);

  // 加载历史记录（分页）
  const loadHistory = useCallback(async (page = 1, append = false) => {
    try {
      updateField('historyLoading', true);
      
      // 获取总数
      const totalCount = await getHistoryRecordsCount();
      
      // 分页加载
      const records = await getHistoryRecordsPaginated(page, state.historyPageSize);
      
      // 从 IndexedDB 加载图片 URL
      const recordsWithImages = await Promise.all(
        records.map(async (record) => {
          if (record.imageIds && record.imageIds.length > 0) {
            const { getImageFromCache } = await import('../../utils/imageCache');
            const images = await Promise.all(
              record.imageIds.map(async (imageId) => {
                const cachedImage = await getImageFromCache(imageId);
                if (cachedImage) {
                  return { url: cachedImage.url || cachedImage.objectUrl };
                }
                return null;
              })
            );
            
            // 过滤掉加载失败的图片
            const validImages = images.filter(img => img !== null);
            
            return {
              ...record,
              images: validImages.length > 0 ? validImages : []
            };
          }
          return record;
        })
      );
      
      updateFields({
        historyRecords: append ? [...state.historyRecords, ...recordsWithImages] : recordsWithImages,
        historyPage: page,
        historyTotalCount: totalCount,
        historyHasMore: page * state.historyPageSize < totalCount,
        historyLoading: false,
      });
    } catch (e) {
      console.error('Failed to load history:', e);
      updateField('historyLoading', false);
    }
  }, [updateField, updateFields, state.historyPageSize, state.historyRecords]);

  // 加载更多历史记录
  const loadMoreHistory = useCallback(async () => {
    if (!state.historyHasMore || state.historyLoading) {
      return;
    }
    await loadHistory(state.historyPage + 1, true);
  }, [state.historyHasMore, state.historyLoading, state.historyPage, loadHistory]);

  // 搜索历史记录（搜索全部）
  const searchHistory = useCallback(async (searchText) => {
    try {
      updateField('historyLoading', true);
      
      // 搜索所有记录
      const records = await searchHistoryRecords(searchText);
      
      // 从 IndexedDB 加载图片 URL
      const recordsWithImages = await Promise.all(
        records.map(async (record) => {
          if (record.imageIds && record.imageIds.length > 0) {
            const { getImageFromCache } = await import('../../utils/imageCache');
            const images = await Promise.all(
              record.imageIds.map(async (imageId) => {
                const cachedImage = await getImageFromCache(imageId);
                if (cachedImage) {
                  return { url: cachedImage.url || cachedImage.objectUrl };
                }
                return null;
              })
            );
            
            // 过滤掉加载失败的图片
            const validImages = images.filter(img => img !== null);
            
            return {
              ...record,
              images: validImages.length > 0 ? validImages : []
            };
          }
          return record;
        })
      );
      
      updateFields({
        historyRecords: recordsWithImages,
        historyHasMore: false, // 搜索结果不分页
        historyLoading: false,
      });
    } catch (e) {
      console.error('Failed to search history:', e);
      updateField('historyLoading', false);
    }
  }, [updateField, updateFields]);

  // 初始加载历史记录
  useEffect(() => {
    loadHistory(1, false);
  }, []);

  // 加载缓存统计
  const loadCacheStats = useCallback(async () => {
    const stats = await getCacheStats();
    updateField('cacheStats', stats);
  }, [updateField]);

  // 初始加载缓存统计
  useEffect(() => {
    loadCacheStats();
  }, [loadCacheStats]);

  // 保存历史记录（移除 localStorage，完全使用 IndexedDB）
  // 此函数已不再需要，直接使用 saveHistoryRecord
  
  // 添加历史记录
  const addHistoryRecord = useCallback(
    async (record) => {
      // 保存到 IndexedDB
      await saveHistoryRecord(record);
      
      // 更新状态 - 添加到列表开头
      setState((prev) => {
        const newRecords = [record, ...prev.historyRecords];
        return { 
          ...prev, 
          historyRecords: newRecords,
          historyTotalCount: prev.historyTotalCount + 1,
        };
      });
    },
    []
  );

  // 删除历史记录
  const deleteHistoryRecordHandler = useCallback(
    async (id) => {
      // 先从 IndexedDB 删除图片
      const record = state.historyRecords.find(r => r.id === id);
      if (record && record.imageIds) {
        const { deleteImageFromCache } = await import('../../utils/imageCache');
        for (const imageId of record.imageIds) {
          await deleteImageFromCache(imageId);
        }
      }
      
      // 从 IndexedDB 删除历史记录
      await deleteHistoryFromDB(id);
      
      // 更新状态
      setState((prev) => ({
        ...prev,
        historyRecords: prev.historyRecords.filter((r) => r.id !== id),
        historyTotalCount: Math.max(0, prev.historyTotalCount - 1),
      }));
      
      // 更新缓存统计
      await loadCacheStats();
    },
    [state.historyRecords, loadCacheStats]
  );

  // 清空历史记录
  const clearHistoryHandler = useCallback(async () => {
    // 先从 IndexedDB 删除所有图片
    const { clearAllCache } = await import('../../utils/imageCache');
    await clearAllCache();
    
    // 清空 IndexedDB 中的历史记录
    await clearAllHistory();
    
    // 更新状态
    setState((prev) => ({
      ...prev,
      historyRecords: [],
      historyPage: 1,
      historyHasMore: false,
      historyTotalCount: 0,
    }));
    
    // 更新缓存统计
    await loadCacheStats();
  }, [loadCacheStats]);

  // 从历史记录加载
  const loadFromHistory = useCallback(
    (record) => {
      updateFields({
        prompt: record.prompt || '',
        negativePrompt: record.negativePrompt || '',
        selectedModel: record.model || state.availableModels[0]?.value || '',
        resolution: record.params?.resolution || '1k',
        aspectRatio: record.params?.aspectRatio || '1:1',
        numberOfImages: record.params?.numberOfImages || 1,
        referenceImages: record.referenceImages || [],
      });
    },
    [updateFields, state.availableModels]
  );

  // 判断是否为 Gemini 图像模型
  const isGeminiImageModel = useCallback((modelName) => {
    if (!modelName) return false;
    const lowerModel = modelName.toLowerCase();
    return lowerModel.includes('gemini') && lowerModel.includes('image');
  }, []);

  // 生成图像
  const generateImage = useCallback(async () => {
    if (!state.prompt.trim()) {
      Toast.warning('请输入提示词');
      return;
    }

    if (!state.selectedModel) {
      Toast.warning('请选择模型');
      return;
    }

    if (!state.selectedToken) {
      Toast.warning('请选择令牌');
      return;
    }

    updateFields({
      generationStatus: GENERATION_STATUS.LOADING,
      generationError: null,
      generationStartTime: Date.now(),
      retryMessage: null, // 清空重试消息
    });

    const MAX_RETRIES = 15;
    let retryCount = 0;
    let shownRetryMessage = false; // 使用局部变量跟踪本次生成是否已显示提示

    const attemptGeneration = async () => {
      try {
        const { width, height } = currentSize;
        const serverAddress = getServerAddress();
        const tokenKey = state.selectedToken.key || state.selectedToken.value;
        const isGemini = isGeminiImageModel(state.selectedModel);

        let res;
        let images = [];

        if (isGemini) {
        // Gemini 图像模型使用 Gemini 原生格式接口
        // 接口: /v1beta/models/{model}:generateContent

        // 根据比例转换为 Gemini 支持的格式
        const aspectRatioMap = {
          '1:1': '1:1',
          '16:9': '16:9',
          '9:16': '9:16',
          '4:3': '4:3',
          '3:4': '3:4',
          '3:2': '3:2',
          '2:3': '2:3',
          '21:9': '21:9',
        };

        // 根据分辨率转换为 Gemini 支持的格式
        const imageSizeMap = {
          '1k': '1K',
          '2k': '2K',
          '4k': '4K',
        };

        // 构建 parts 数组
        const parts = [];

        // 添加参考图片（如果有）
        if (state.referenceImages && state.referenceImages.length > 0) {
          state.referenceImages.forEach((img) => {
            // 从 data URL 中提取 base64 数据和 mime type
            const matches = img.url.match(/^data:([^;]+);base64,(.+)$/);
            if (matches) {
              const mimeType = matches[1];
              const base64Data = matches[2];
              parts.push({
                inlineData: {
                  mimeType: mimeType,
                  data: base64Data,
                },
              });
            }
          });
        }

        // 添加文本提示词
        parts.push({
          text: state.prompt,
        });

        const payload = {
          contents: [
            {
              parts: parts,
            },
          ],
          generationConfig: {
            responseModalities: ['IMAGE'],
            imageConfig: {
              aspectRatio: aspectRatioMap[state.aspectRatio] || '1:1',
              imageSize: imageSizeMap[state.resolution] || '1K',
            },
          },
        };

        const apiUrl = `/v1beta/models/${state.selectedModel}:generateContent`;

        // 使用系统封装的 API 发送请求
        const response = await API.post(apiUrl, payload, {
          headers: {
            Authorization: `Bearer sk-${tokenKey}`,
          },
        });

        const result = response.data;

        // 从 Gemini 原生响应中提取图像
        // 响应格式: candidates[].content.parts[].inlineData 或 candidates[].content.parts[].text
        if (result.candidates && result.candidates.length > 0) {
          result.candidates.forEach((candidate, index) => {
            if (candidate.content && candidate.content.parts) {
              candidate.content.parts.forEach((part, subIndex) => {
                // 处理 inlineData 格式
                if (part.inlineData && part.inlineData.mimeType && part.inlineData.data) {
                  const mimeType = part.inlineData.mimeType;
                  let base64Data = part.inlineData.data;
                  
                  // 如果 base64 数据已经包含 data: 前缀，直接使用
                  // 否则构建完整的 data URL
                  let imageUrl;
                  if (base64Data.startsWith('data:')) {
                    imageUrl = base64Data;
                  } else {
                    // 确保 base64 数据不包含换行符和空格
                    base64Data = base64Data.replace(/\s/g, '');
                    imageUrl = `data:${mimeType};base64,${base64Data}`;
                  }
                  
                  images.push({
                    id: `${Date.now()}-${index}-${subIndex}`,
                    url: imageUrl,
                    revisedPrompt: null,
                  });
                }
                // 处理 text 格式（markdown 图像）
                else if (part.text && typeof part.text === 'string') {
                  // 匹配 markdown 格式: ![image](data:image/jpeg;base64,...)
                  const markdownImageRegex = /!\[.*?\]\((data:image\/[^;]+;base64,[^)]+)\)/g;
                  let match;
                  while ((match = markdownImageRegex.exec(part.text)) !== null) {
                    images.push({
                      id: `${Date.now()}-${index}-${subIndex}-${images.length}`,
                      url: match[1],
                      revisedPrompt: null,
                    });
                  }
                }
              });
            }
          });
        }
      } else {
        // 其他模型使用 Images Generations API
        const payload = {
          model: state.selectedModel,
          prompt: state.prompt,
          n: state.numberOfImages,
          size: `${width}x${height}`,
          response_format: 'url',
        };

        // 如果有反向提示词
        if (state.negativePrompt.trim()) {
          payload.negative_prompt = state.negativePrompt;
        }

        // 如果有参考图片，添加到 payload
        if (state.referenceImages && state.referenceImages.length > 0) {
          // 将参考图片转换为 base64 数组
          payload.reference_images = state.referenceImages.map((img) => {
            // 如果已经是 data URL，直接返回
            if (img.url.startsWith('data:')) {
              return img.url;
            }
            return img.url;
          });
        }

        res = await fetch(`${serverAddress}/v1/images/generations`, {
          method: 'POST',
          headers: {
            Authorization: `Bearer sk-${tokenKey}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(payload),
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          const error = new Error(
            errorData.error?.message || errorData.message || `HTTP error! status: ${res.status}`
          );
          error.response = { status: res.status, data: errorData };
          throw error;
        }

        const result = await res.json();
        const data = result.data;

        if (data && data.length > 0) {
          images = data.map((item, index) => ({
            id: `${Date.now()}-${index}`,
            url: item.url || `data:image/png;base64,${item.b64_json}`,
            revisedPrompt: item.revised_prompt,
          }));
        }
      }

      if (images.length > 0) {
        // 保存图片到 IndexedDB
        const recordId = Date.now().toString();
        const imageIds = [];
        for (let i = 0; i < images.length; i++) {
          const img = images[i];
          const imageId = `${recordId}-${i}`;
          await saveImageToCache(imageId, img.url, {
            prompt: state.prompt,
            model: state.selectedModel,
            timestamp: Date.now(),
          });
          imageIds.push(imageId);
        }

        // 添加到历史记录 - 只保存图片 ID，不保存 URL
        const historyRecord = {
          id: recordId,
          timestamp: Date.now(),
          prompt: state.prompt,
          negativePrompt: state.negativePrompt,
          model: state.selectedModel,
          params: {
            resolution: state.resolution,
            aspectRatio: state.aspectRatio,
            numberOfImages: state.numberOfImages,
            width,
            height,
          },
          referenceImages: state.referenceImages.map((img) => ({
            id: img.id,
            name: img.name,
          })),
          imageIds: imageIds, // 保存图片 ID
          images: images, // 临时保存在内存中用于显示，不会持久化到 localStorage
          status: 'success',
        };
        addHistoryRecord(historyRecord);

        // 更新缓存统计
        await loadCacheStats();

        updateFields({
          generationStatus: GENERATION_STATUS.SUCCESS,
          generatedImages: images,
          selectedImageIndex: 0,
          generationStartTime: null,
          retryMessage: null, // 清空重试消息
        });

        Toast.success('图像生成成功');
      } else {
        throw new Error('未返回图像数据');
      }
      } catch (error) {
        console.error('Image generation failed:', error);
        
        // 更精准的错误信息提取
        let errorMessage = '图像生成失败';
        let statusCode = null;
        let errorCode = null;
        
        if (error.response) {
          // HTTP 响应错误
          statusCode = error.response.status;
          const data = error.response.data;
          
          // 提取错误代码
          if (data?.error?.code) {
            errorCode = data.error.code;
          } else if (data?.code) {
            errorCode = data.code;
          }
          
          if (data?.error?.message) {
            errorMessage = data.error.message;
          } else if (data?.message) {
            errorMessage = data.message;
          } else if (data?.error) {
            errorMessage = typeof data.error === 'string' ? data.error : JSON.stringify(data.error);
          } else {
            errorMessage = `请求失败 (${statusCode})`;
          }
        } else if (error.message) {
          // 网络错误或其他错误
          if (error.message.includes('Failed to fetch') || error.message.includes('Network')) {
            errorMessage = '网络连接失败，请检查网络设置';
          } else if (error.message.includes('timeout')) {
            errorMessage = '请求超时，请稍后重试';
          } else {
            errorMessage = error.message;
          }
        }

        // 如果是 model_not_found 错误，不重试
        // if (errorCode === 'model_not_found') {
        //   updateFields({
        //     generationStatus: GENERATION_STATUS.ERROR,
        //     generationError: errorMessage,
        //     generationStartTime: null,
        //     retryMessage: null,
        //   });
        //   Toast.error(errorMessage);
        //   return;
        // }

        // 检查是否为 500x 错误，如果是则重试
        if (statusCode && statusCode >= 500 && statusCode < 600 && retryCount < MAX_RETRIES) {
          retryCount++;
          
          // 设置重试提示信息（持续显示）
          if (!shownRetryMessage) {
            updateField('retryMessage', '由于官方负载太高，生图时间有所延长，请耐心等待');
            shownRetryMessage = true;
          }
          
          console.log(`遇到 ${statusCode} 错误，正在进行第 ${retryCount}/${MAX_RETRIES} 次重试...`);
          
          // 等待一段时间后重试（指数退避）
          const delay = Math.min(1000 * Math.pow(2, retryCount - 1), 10000);
          await new Promise(resolve => setTimeout(resolve, delay));
          
          return attemptGeneration();
        }

        // 达到最大重试次数或非 500x 错误
        if (retryCount >= MAX_RETRIES) {
          errorMessage = `${errorMessage} (已重试 ${MAX_RETRIES} 次)`;
        }

        updateFields({
          generationStatus: GENERATION_STATUS.ERROR,
          generationError: errorMessage,
          generationStartTime: null,
          retryMessage: null, // 清空重试消息
        });

        // Toast.error(errorMessage);
      }
    };

    // 开始生成
    await attemptGeneration();
  }, [
    state.prompt,
    state.negativePrompt,
    state.selectedModel,
    state.selectedToken,
    state.numberOfImages,
    state.resolution,
    state.aspectRatio,
    state.referenceImages,
    currentSize,
    updateField,
    updateFields,
    addHistoryRecord,
    loadCacheStats,
    isGeminiImageModel,
  ]);

  // 重置生成状态
  const resetGeneration = useCallback(() => {
    updateFields({
      generationStatus: GENERATION_STATUS.IDLE,
      generationError: null,
      generatedImages: [],
      selectedImageIndex: 0,
      generationStartTime: null,
    });
  }, [updateFields]);

  return {
    // 状态
    ...state,
    currentSize,

    // 更新方法
    updateField,
    updateFields,

    // 操作方法
    loadTokens,
    handleTokenChange,
    loadModelsForToken,
    generateImage,
    resetGeneration,

    // 历史记录方法
    addHistoryRecord,
    deleteHistoryRecord: deleteHistoryRecordHandler,
    clearHistory: clearHistoryHandler,
    loadFromHistory,
    loadHistory,
    loadMoreHistory,
    searchHistory,

    // 缓存方法
    loadCacheStats,
  };
};