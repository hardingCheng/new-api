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

// IndexedDB 配置
const DB_NAME = 'BananaImageCache';
const DB_VERSION = 2; // 升级版本以添加新的 store
const STORE_NAME = 'images';
const HISTORY_STORE_NAME = 'history'; // 新增：历史记录 store

// 默认清理配置
export const DEFAULT_CACHE_CONFIG = {
  maxAge: 7 * 24 * 60 * 60 * 1000, // 7天
  maxCount: 100, // 最多100张图片
  maxSize: 50 * 1024 * 1024, // 50MB
};

// 获取缓存配置
export const getCacheConfig = () => {
  try {
    const config = localStorage.getItem('banana_image_cache_config');
    return config ? JSON.parse(config) : DEFAULT_CACHE_CONFIG;
  } catch (e) {
    console.error('Failed to load cache config:', e);
    return DEFAULT_CACHE_CONFIG;
  }
};

// 保存缓存配置
export const saveCacheConfig = (config) => {
  try {
    localStorage.setItem('banana_image_cache_config', JSON.stringify(config));
  } catch (e) {
    console.error('Failed to save cache config:', e);
  }
};

// 打开数据库
const openDB = () => {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);

    request.onerror = () => reject(request.error);
    request.onsuccess = () => resolve(request.result);

    request.onupgradeneeded = (event) => {
      const db = event.target.result;
      
      // 图片存储
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        const store = db.createObjectStore(STORE_NAME, { keyPath: 'id' });
        store.createIndex('timestamp', 'timestamp', { unique: false });
        store.createIndex('size', 'size', { unique: false });
      }
      
      // 历史记录存储
      if (!db.objectStoreNames.contains(HISTORY_STORE_NAME)) {
        const historyStore = db.createObjectStore(HISTORY_STORE_NAME, { keyPath: 'id' });
        historyStore.createIndex('timestamp', 'timestamp', { unique: false });
      }
    };
  });
};

// 将 URL 转换为 Blob
const urlToBlob = async (url) => {
  if (url.startsWith('data:')) {
    // Base64 数据
    const response = await fetch(url);
    return await response.blob();
  } else {
    // 网络 URL
    const response = await fetch(url);
    return await response.blob();
  }
};

// 保存图片到 IndexedDB
export const saveImageToCache = async (id, url, metadata = {}) => {
  try {
    const db = await openDB();
    const blob = await urlToBlob(url);
    
    const imageData = {
      id,
      blob,
      url, // 保留原始 URL 用于显示
      size: blob.size,
      timestamp: Date.now(),
      metadata,
    };

    const transaction = db.transaction([STORE_NAME], 'readwrite');
    const store = transaction.objectStore(STORE_NAME);
    await store.put(imageData);

    // 检查是否需要清理
    await cleanupCache();

    return true;
  } catch (error) {
    console.error('Failed to save image to cache:', error);
    return false;
  }
};

// 从 IndexedDB 获取图片
export const getImageFromCache = async (id) => {
  try {
    const db = await openDB();
    const transaction = db.transaction([STORE_NAME], 'readonly');
    const store = transaction.objectStore(STORE_NAME);
    
    return new Promise((resolve, reject) => {
      const request = store.get(id);
      request.onsuccess = () => {
        const result = request.result;
        if (result && result.blob) {
          // 将 Blob 转换为 URL
          result.objectUrl = URL.createObjectURL(result.blob);
        }
        resolve(result);
      };
      request.onerror = () => reject(request.error);
    });
  } catch (error) {
    console.error('Failed to get image from cache:', error);
    return null;
  }
};

// 获取所有缓存的图片
export const getAllCachedImages = async () => {
  try {
    const db = await openDB();
    const transaction = db.transaction([STORE_NAME], 'readonly');
    const store = transaction.objectStore(STORE_NAME);
    
    return new Promise((resolve, reject) => {
      const request = store.getAll();
      request.onsuccess = () => resolve(request.result || []);
      request.onerror = () => reject(request.error);
    });
  } catch (error) {
    console.error('Failed to get all cached images:', error);
    return [];
  }
};

// 删除缓存的图片
export const deleteImageFromCache = async (id) => {
  try {
    const db = await openDB();
    const transaction = db.transaction([STORE_NAME], 'readwrite');
    const store = transaction.objectStore(STORE_NAME);
    await store.delete(id);
    return true;
  } catch (error) {
    console.error('Failed to delete image from cache:', error);
    return false;
  }
};

// 清空所有缓存
export const clearAllCache = async () => {
  try {
    const db = await openDB();
    const transaction = db.transaction([STORE_NAME], 'readwrite');
    const store = transaction.objectStore(STORE_NAME);
    await store.clear();
    return true;
  } catch (error) {
    console.error('Failed to clear cache:', error);
    return false;
  }
};

// 获取缓存统计信息
export const getCacheStats = async () => {
  try {
    const images = await getAllCachedImages();
    const totalSize = images.reduce((sum, img) => sum + (img.size || 0), 0);
    const oldestTimestamp = images.length > 0 
      ? Math.min(...images.map(img => img.timestamp))
      : Date.now();
    
    return {
      count: images.length,
      totalSize,
      oldestTimestamp,
    };
  } catch (error) {
    console.error('Failed to get cache stats:', error);
    return {
      count: 0,
      totalSize: 0,
      oldestTimestamp: Date.now(),
    };
  }
};

// 清理缓存（根据配置）
export const cleanupCache = async () => {
  try {
    const config = getCacheConfig();
    const images = await getAllCachedImages();
    
    if (images.length === 0) return;

    const now = Date.now();
    const db = await openDB();
    const transaction = db.transaction([STORE_NAME], 'readwrite');
    const store = transaction.objectStore(STORE_NAME);

    // 按时间排序（最旧的在前）
    images.sort((a, b) => a.timestamp - b.timestamp);

    let toDelete = [];

    // 1. 按时间清理
    if (config.maxAge) {
      const expiredImages = images.filter(img => now - img.timestamp > config.maxAge);
      toDelete.push(...expiredImages.map(img => img.id));
    }

    // 2. 按数量清理
    if (config.maxCount && images.length > config.maxCount) {
      const excessCount = images.length - config.maxCount;
      const excessImages = images.slice(0, excessCount);
      toDelete.push(...excessImages.map(img => img.id));
    }

    // 3. 按存储大小清理
    if (config.maxSize) {
      let totalSize = images.reduce((sum, img) => sum + (img.size || 0), 0);
      let index = 0;
      
      while (totalSize > config.maxSize && index < images.length) {
        const img = images[index];
        toDelete.push(img.id);
        totalSize -= img.size || 0;
        index++;
      }
    }

    // 去重并删除
    const uniqueToDelete = [...new Set(toDelete)];
    for (const id of uniqueToDelete) {
      await store.delete(id);
    }

    if (uniqueToDelete.length > 0) {
      console.log(`Cleaned up ${uniqueToDelete.length} cached images`);
    }

    return uniqueToDelete.length;
  } catch (error) {
    console.error('Failed to cleanup cache:', error);
    return 0;
  }
};

// 下载图片
export const downloadImage = async (url, filename = 'image.png') => {
  try {
    let blob;
    
    if (url.startsWith('data:')) {
      // Base64 数据
      const response = await fetch(url);
      blob = await response.blob();
    } else if (url.startsWith('blob:')) {
      // Blob URL
      const response = await fetch(url);
      blob = await response.blob();
    } else {
      // 网络 URL
      const response = await fetch(url);
      blob = await response.blob();
    }

    // 创建下载链接
    const downloadUrl = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = downloadUrl;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    
    // 清理 URL
    setTimeout(() => URL.revokeObjectURL(downloadUrl), 100);
    
    return true;
  } catch (error) {
    console.error('Failed to download image:', error);
    return false;
  }
};

// ==================== 历史记录管理 ====================

// 保存历史记录
export const saveHistoryRecord = async (record) => {
  try {
    const db = await openDB();
    const transaction = db.transaction([HISTORY_STORE_NAME], 'readwrite');
    const store = transaction.objectStore(HISTORY_STORE_NAME);
    await store.put(record);
    return true;
  } catch (error) {
    console.error('Failed to save history record:', error);
    return false;
  }
};

// 获取所有历史记录
export const getAllHistoryRecords = async () => {
  try {
    const db = await openDB();
    const transaction = db.transaction([HISTORY_STORE_NAME], 'readonly');
    const store = transaction.objectStore(HISTORY_STORE_NAME);
    
    return new Promise((resolve, reject) => {
      const request = store.getAll();
      request.onsuccess = () => {
        const records = request.result || [];
        // 按时间倒序排序（最新的在前）
        records.sort((a, b) => b.timestamp - a.timestamp);
        resolve(records);
      };
      request.onerror = () => reject(request.error);
    });
  } catch (error) {
    console.error('Failed to get history records:', error);
    return [];
  }
};

// 删除历史记录
export const deleteHistoryRecord = async (id) => {
  try {
    const db = await openDB();
    const transaction = db.transaction([HISTORY_STORE_NAME], 'readwrite');
    const store = transaction.objectStore(HISTORY_STORE_NAME);
    await store.delete(id);
    return true;
  } catch (error) {
    console.error('Failed to delete history record:', error);
    return false;
  }
};

// 清空所有历史记录
export const clearAllHistory = async () => {
  try {
    const db = await openDB();
    const transaction = db.transaction([HISTORY_STORE_NAME], 'readwrite');
    const store = transaction.objectStore(HISTORY_STORE_NAME);
    await store.clear();
    return true;
  } catch (error) {
    console.error('Failed to clear history:', error);
    return false;
  }
};

