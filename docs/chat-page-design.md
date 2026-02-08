# AI 对话页面设计方案

## 一、概述

基于现有系统架构，设计一个功能完整的 AI 对话页面，支持多模型对话、流式响应、图片上传、历史记录管理等功能。

## 二、功能需求

### 2.1 核心功能
- ✅ 令牌选择（用户可选择不同的 API 令牌）
- ✅ 多模型选择（根据所选令牌动态加载可用模型）
- ✅ 实时流式对话
- ✅ 多轮对话历史
- ✅ 图片上传与识别
- ✅ Markdown 渲染
- ✅ 代码高亮显示
- ✅ 消息编辑与重新生成
- ✅ 对话导出（JSON、Markdown）
- ✅ 系统提示词设置

### 2.2 高级功能
- ✅ 对话历史管理（新建、保存、加载、删除）
- ✅ Token 使用统计
- ✅ 响应时间监控
- ✅ 错误处理与重试
- ✅ 快捷键支持
- ✅ 移动端适配

## 三、技术架构

### 3.1 技术栈
- **前端框架**: React 18
- **UI 组件库**: Semi Design (@douyinfe/semi-ui)
- **状态管理**: React Hooks (useState, useContext, useReducer)
- **路由**: React Router v6
- **HTTP 客户端**: Axios
- **Markdown 渲染**: react-markdown + rehype-highlight
- **代码高亮**: highlight.js
- **国际化**: react-i18next
- **样式**: Tailwind CSS

### 3.2 API 接口

#### 3.2.1 聊天对话接口
```
POST /v1/chat/completions
```

**请求参数**:
```json
{
  "model": "gpt-4",
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful assistant."
    },
    {
      "role": "user",
      "content": "Hello!"
    }
  ],
  "stream": true
}
```

**请求头**:
```
Authorization: Bearer {API令牌key}
Content-Type: application/json
```

**响应格式（流式）**:
```
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

#### 3.2.2 令牌列表接口
```
GET /api/token
```

**请求头**:
```
Authorization: Bearer {用户登录token}
```

**响应格式**:
```json
{
  "success": true,
  "message": "",
  "data": {
    "items": [
      {
        "id": 1,
        "name": "我的API令牌",
        "key": "sk-xxx",
        "status": 1,
        "remain_quota": 1000000,
        "unlimited_quota": false,
        "model_limits_enabled": true,
        "model_limits": "gpt-4,gpt-3.5-turbo,claude-3",
        "group": "default",
        "expired_time": -1,
        "created_time": 1234567890,
        "accessed_time": 1234567890
      }
    ],
    "total": 10,
    "page": 1,
    "page_size": 20
  }
}
```

#### 3.2.3 模型列表接口（根据令牌）
```
GET /v1/models
```

**请求头**:
```
Authorization: Bearer {API令牌key}
```

**响应格式**:
```json
{
  "success": true,
  "object": "list",
  "data": [
    {
      "id": "gpt-4",
      "object": "model",
      "created": 1626777600,
      "owned_by": "openai",
      "supported_endpoint_types": [1, 2, 3]
    },
    {
      "id": "claude-3-opus",
      "object": "model",
      "created": 1626777600,
      "owned_by": "anthropic",
      "supported_endpoint_types": [1]
    }
  ]
}
```

**说明**:
- 该接口会根据请求头中的 API 令牌返回该令牌可用的模型列表
- 如果令牌启用了模型限制（`model_limits_enabled: true`），则只返回 `model_limits` 中指定的模型
- 如果令牌未启用模型限制，则返回该令牌所属分组（`group`）下的所有可用模型

## 四、页面结构设计

### 4.1 整体布局

```
┌─────────────────────────────────────────────────────────┐
│                      顶部导航栏                          │
├──────────────┬──────────────────────────┬───────────────┤
│              │                          │               │
│   侧边栏     │      对话主区域          │   设置面板    │
│  (可折叠)    │                          │   (可折叠)    │
│              │                          │               │
│  - 新建对话  │  ┌──────────────────┐   │  - 令牌选择   │
│  - 历史记录  │  │   消息列表区域   │   │  - 模型选择   │
│  - 收藏对话  │  │                  │   │  - 系统提示   │
│              │  │  [用户消息]      │   │  - 图片上传   │
│              │  │  [AI回复]        │   │               │
│              │  │  [用户消息]      │   │               │
│              │  │  [AI回复...]     │   │               │
│              │  └──────────────────┘   │               │
│              │  ┌──────────────────┐   │               │
│              │  │   输入框区域     │   │               │
│              │  │  [文本输入框]    │   │               │
│              │  │  [发送按钮]      │   │               │
│              │  └──────────────────┘   │               │
│              │                          │               │
└──────────────┴──────────────────────────┴───────────────┘
```

### 4.2 组件层级结构

```
ChatPage (页面容器)
├── ChatSidebar (侧边栏)
│   ├── NewChatButton (新建对话按钮)
│   ├── ChatHistoryList (历史记录列表)
│   │   └── ChatHistoryItem (历史记录项)
│   └── ChatActions (操作按钮组)
│
├── ChatMainArea (主对话区域)
│   ├── ChatHeader (对话头部)
│   │   ├── TokenSelector (令牌选择器)
│   │   ├── ModelSelector (模型选择器)
│   │   └── ChatActions (操作按钮)
│   │
│   ├── MessageList (消息列表)
│   │   └── MessageItem (消息项)
│   │       ├── MessageAvatar (头像)
│   │       ├── MessageContent (内容)
│   │       │   ├── MarkdownRenderer (Markdown渲染)
│   │       │   ├── CodeBlock (代码块)
│   │       │   └── ImageViewer (图片查看)
│   │       └── MessageActions (消息操作)
│   │           ├── CopyButton (复制)
│   │           ├── EditButton (编辑)
│   │           ├── RegenerateButton (重新生成)
│   │           └── DeleteButton (删除)
│   │
│   └── ChatInput (输入区域)
│       ├── TextArea (文本输入框)
│       ├── ImageUpload (图片上传)
│       ├── SendButton (发送按钮)
│       └── StopButton (停止生成)
│
└── ChatSettingsPanel (设置面板)
    ├── TokenSelector (令牌选择)
    ├── ModelSettings (模型设置)
    ├── SystemPrompt (系统提示词)
    └── AdvancedSettings (高级设置)
```

## 五、数据流设计

### 5.1 状态管理

```javascript
// 全局状态
const ChatContext = {
  // 当前对话
  currentChat: {
    id: string,
    title: string,
    tokenId: number,
    tokenKey: string,
    model: string,
    messages: Message[],
    createdAt: timestamp,
    updatedAt: timestamp,
  },
  
  // 对话列表
  chatHistory: Chat[],
  
  // 令牌列表
  tokens: Token[],
  
  // 当前选中的令牌
  selectedToken: Token | null,
  
  // 可用模型列表（根据选中的令牌动态加载）
  availableModels: Model[],
  
  // 模型配置
  modelConfig: {
    model: string,
    stream: boolean,
  },
  
  // 系统提示词
  systemPrompt: string,
  
  // UI 状态
  uiState: {
    isLoading: boolean,
    isSidebarOpen: boolean,
    isSettingsPanelOpen: boolean,
    error: string | null,
  },
  
  // 统计信息
  stats: {
    totalTokens: number,
    promptTokens: number,
    completionTokens: number,
    responseTime: number,
  },
}
```

### 5.2 消息数据结构

```javascript
interface Token {
  id: number;
  name: string;
  key: string;
  status: number; // 1: 启用, 2: 禁用, 3: 已过期, 4: 已用尽
  remain_quota: number;
  unlimited_quota: boolean;
  model_limits_enabled: boolean;
  model_limits: string; // 逗号分隔的模型列表
  group: string;
  expired_time: number; // -1 表示永不过期
  created_time: number;
  accessed_time: number;
}

interface Model {
  id: string;
  object: string;
  created: number;
  owned_by: string;
  supported_endpoint_types?: number[];
}

interface Message {
  id: string;
  role: 'system' | 'user' | 'assistant';
  content: string | MessageContent[];
  timestamp: number;
  status?: 'sending' | 'success' | 'error' | 'loading';
  error?: string;
  tokens?: {
    prompt: number;
    completion: number;
    total: number;
  };
}

interface MessageContent {
  type: 'text' | 'image_url';
  text?: string;
  image_url?: {
    url: string;
    detail?: 'auto' | 'low' | 'high';
  };
}
```

## 六、核心功能实现

### 6.1 令牌与模型工作流程

#### 6.1.1 工作流程说明

```
用户登录
  ↓
加载用户令牌列表 (GET /api/token)
  ↓
选择一个令牌
  ↓
根据令牌获取可用模型 (GET /v1/models with Bearer token)
  ↓
选择模型
  ↓
发起对话 (POST /v1/chat/completions with Bearer token)
```

#### 6.1.2 令牌与模型的关系

1. **令牌限制模型**：
   - 如果令牌启用了 `model_limits_enabled: true`
   - 则只能使用 `model_limits` 字段中指定的模型
   - 例如：`"model_limits": "gpt-4,gpt-3.5-turbo,claude-3"`

2. **令牌分组模型**：
   - 如果令牌未启用模型限制
   - 则可以使用该令牌所属分组（`group` 字段）下的所有可用模型
   - 系统会根据分组返回该分组下所有已启用的模型

3. **令牌额度**：
   - `unlimited_quota: true` 表示无限额度
   - `unlimited_quota: false` 时，`remain_quota` 表示剩余额度
   - 额度不足时应提示用户

#### 6.1.3 实现要点

```javascript
// 1. 页面加载时获取令牌列表
useEffect(() => {
  fetchUserTokens();
}, []);

// 2. 选择令牌后立即获取该令牌的可用模型
const handleTokenChange = async (token) => {
  setSelectedToken(token);
  const models = await fetchModelsForToken(token.key);
  setAvailableModels(models);
  
  // 如果当前选中的模型不在新的模型列表中，清空模型选择
  if (selectedModel && !models.find(m => m.id === selectedModel)) {
    setSelectedModel(null);
  }
};

// 3. 发送消息时使用选中令牌的 key
const sendMessage = async (content) => {
  if (!selectedToken) {
    showError('请先选择 API 令牌');
    return;
  }
  
  if (!selectedModel) {
    showError('请先选择模型');
    return;
  }
  
  await fetch('/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${selectedToken.key}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: selectedModel,
      messages: [...],
      stream: true,
    }),
  });
};

// 4. 创建新对话时保存令牌和模型信息
const createNewChat = () => {
  const newChat = {
    id: generateId(),
    tokenId: selectedToken.id,
    tokenKey: selectedToken.key,
    model: selectedModel,
    messages: [],
    // ...
  };
  saveChat(newChat);
};

// 5. 加载历史对话时恢复令牌和模型
const loadChat = (chat) => {
  // 检查令牌是否还存在且可用
  const token = tokens.find(t => t.id === chat.tokenId);
  if (!token) {
    showWarning('该对话使用的令牌已不可用，请选择新的令牌');
    return;
  }
  
  setSelectedToken(token);
  setSelectedModel(chat.model);
  setCurrentChat(chat);
};
```

### 6.2 流式对话实现

```javascript
// hooks/useStreamChat.js
import { useState, useCallback, useRef } from 'react';
import axios from 'axios';

export const useStreamChat = () => {
  const [isStreaming, setIsStreaming] = useState(false);
  const abortControllerRef = useRef(null);

  const sendMessage = useCallback(async (messages, config, onChunk, onComplete, onError) => {
    setIsStreaming(true);
    abortControllerRef.current = new AbortController();

    try {
      const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${config.tokenKey}`, // 使用选中令牌的 key
        },
        body: JSON.stringify({
          messages,
          model: config.model,
          stream: true,
        }),
        signal: abortControllerRef.current.signal,
      });

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.slice(6);
            if (data === '[DONE]') {
              onComplete?.();
              break;
            }

            try {
              const parsed = JSON.parse(data);
              const content = parsed.choices?.[0]?.delta?.content;
              if (content) {
                onChunk?.(content);
              }
            } catch (e) {
              console.error('Parse error:', e);
            }
          }
        }
      }
    } catch (error) {
      if (error.name !== 'AbortError') {
        onError?.(error);
      }
    } finally {
      setIsStreaming(false);
    }
  }, []);

  const stopStreaming = useCallback(() => {
    abortControllerRef.current?.abort();
    setIsStreaming(false);
  }, []);

  return { sendMessage, stopStreaming, isStreaming };
};
```

### 6.2 令牌和模型管理

```javascript
// hooks/useTokens.js
import { useState, useEffect, useCallback } from 'react';
import axios from 'axios';

export const useTokens = () => {
  const [tokens, setTokens] = useState([]);
  const [selectedToken, setSelectedToken] = useState(null);
  const [availableModels, setAvailableModels] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  // 获取用户的所有令牌
  const fetchTokens = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await axios.get('/api/token', {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('user_token')}`,
        },
        params: {
          page: 1,
          page_size: 100,
        },
      });
      
      if (response.data.success) {
        const activeTokens = response.data.data.items.filter(
          token => token.status === 1 // 只显示启用状态的令牌
        );
        setTokens(activeTokens);
        
        // 如果有令牌且没有选中的令牌，自动选中第一个
        if (activeTokens.length > 0 && !selectedToken) {
          setSelectedToken(activeTokens[0]);
        }
      }
    } catch (err) {
      setError(err.message);
      console.error('Failed to fetch tokens:', err);
    } finally {
      setLoading(false);
    }
  }, [selectedToken]);

  // 根据选中的令牌获取可用模型
  const fetchModels = useCallback(async (token) => {
    if (!token) return;
    
    setLoading(true);
    setError(null);
    try {
      const response = await axios.get('/v1/models', {
        headers: {
          'Authorization': `Bearer ${token.key}`,
        },
      });
      
      if (response.data.success) {
        setAvailableModels(response.data.data || []);
      }
    } catch (err) {
      setError(err.message);
      console.error('Failed to fetch models:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  // 选择令牌
  const selectToken = useCallback((token) => {
    setSelectedToken(token);
    fetchModels(token);
  }, [fetchModels]);

  // 初始化：加载令牌列表
  useEffect(() => {
    fetchTokens();
  }, []);

  // 当选中的令牌变化时，加载对应的模型列表
  useEffect(() => {
    if (selectedToken) {
      fetchModels(selectedToken);
    }
  }, [selectedToken, fetchModels]);

  return {
    tokens,
    selectedToken,
    availableModels,
    loading,
    error,
    selectToken,
    refreshTokens: fetchTokens,
    refreshModels: () => fetchModels(selectedToken),
  };
};
```

```javascript
// components/TokenSelector.jsx
import React from 'react';
import { Select, Tag, Tooltip } from '@douyinfe/semi-ui';

const TokenSelector = ({ tokens, selectedToken, onSelect, disabled }) => {
  const formatQuota = (quota, unlimited) => {
    if (unlimited) return '无限额度';
    return `剩余: ${(quota / 500000).toFixed(2)} 元`;
  };

  const getTokenStatus = (token) => {
    if (token.unlimited_quota) {
      return { color: 'green', text: '无限' };
    }
    if (token.remain_quota <= 0) {
      return { color: 'red', text: '已用尽' };
    }
    if (token.remain_quota < 100000) {
      return { color: 'orange', text: '余额不足' };
    }
    return { color: 'blue', text: '正常' };
  };

  return (
    <Select
      value={selectedToken?.id}
      onChange={(value) => {
        const token = tokens.find(t => t.id === value);
        onSelect(token);
      }}
      disabled={disabled}
      style={{ width: '100%' }}
      placeholder="选择 API 令牌"
      renderSelectedItem={(option) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <span>{option.label}</span>
          <Tag size="small" color={getTokenStatus(selectedToken).color}>
            {getTokenStatus(selectedToken).text}
          </Tag>
        </div>
      )}
    >
      {tokens.map(token => {
        const status = getTokenStatus(token);
        return (
          <Select.Option key={token.id} value={token.id} label={token.name}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <div style={{ fontWeight: 500 }}>{token.name}</div>
                <div style={{ fontSize: '12px', color: '#999' }}>
                  {formatQuota(token.remain_quota, token.unlimited_quota)}
                  {token.model_limits_enabled && (
                    <Tooltip content={`限制模型: ${token.model_limits}`}>
                      <span style={{ marginLeft: '8px', color: '#666' }}>
                        🔒 模型限制
                      </span>
                    </Tooltip>
                  )}
                </div>
              </div>
              <Tag size="small" color={status.color}>
                {status.text}
              </Tag>
            </div>
          </Select.Option>
        );
      })}
    </Select>
  );
};

export default TokenSelector;
```

```javascript
// components/ModelSelector.jsx
import React from 'react';
import { Select, Tag, Empty } from '@douyinfe/semi-ui';

const ModelSelector = ({ models, selectedModel, onSelect, disabled, loading }) => {
  if (loading) {
    return <Select placeholder="加载模型中..." disabled />;
  }

  if (!models || models.length === 0) {
    return (
      <Select 
        placeholder="请先选择令牌" 
        disabled 
        emptyContent={<Empty description="暂无可用模型" />}
      />
    );
  }

  return (
    <Select
      value={selectedModel}
      onChange={onSelect}
      disabled={disabled}
      style={{ width: '100%' }}
      placeholder="选择模型"
      filter
      showClear
    >
      {models.map(model => (
        <Select.Option key={model.id} value={model.id}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <div style={{ fontWeight: 500 }}>{model.id}</div>
              <div style={{ fontSize: '12px', color: '#999' }}>
                {model.owned_by}
              </div>
            </div>
            {model.supported_endpoint_types && model.supported_endpoint_types.length > 0 && (
              <Tag size="small" color="blue">
                {model.supported_endpoint_types.length} 端点
              </Tag>
            )}
          </div>
        </Select.Option>
      ))}
    </Select>
  );
};

export default ModelSelector;
```

### 6.3 对话历史管理

```javascript
// hooks/useChatHistory.js
import { useState, useEffect, useCallback } from 'react';

export const useChatHistory = () => {
  const [chats, setChats] = useState([]);
  const [currentChatId, setCurrentChatId] = useState(null);

  // 从 localStorage 加载历史记录
  useEffect(() => {
    const saved = localStorage.getItem('chat_history');
    if (saved) {
      try {
        setChats(JSON.parse(saved));
      } catch (e) {
        console.error('Failed to load chat history:', e);
      }
    }
  }, []);

  // 保存到 localStorage
  useEffect(() => {
    if (chats.length > 0) {
      localStorage.setItem('chat_history', JSON.stringify(chats));
    }
  }, [chats]);

  // 创建新对话
  const createNewChat = useCallback((tokenId, tokenKey, model = 'gpt-4') => {
    const newChat = {
      id: Date.now().toString(),
      title: '新对话',
      tokenId,
      tokenKey,
      model,
      messages: [],
      createdAt: Date.now(),
      updatedAt: Date.now(),
    };
    setChats(prev => [newChat, ...prev]);
    setCurrentChatId(newChat.id);
    return newChat;
  }, []);

  // 更新对话
  const updateChat = useCallback((chatId, updates) => {
    setChats(prev => prev.map(chat => 
      chat.id === chatId 
        ? { ...chat, ...updates, updatedAt: Date.now() }
        : chat
    ));
  }, []);

  // 删除对话
  const deleteChat = useCallback((chatId) => {
    setChats(prev => prev.filter(chat => chat.id !== chatId));
    if (currentChatId === chatId) {
      setCurrentChatId(null);
    }
  }, [currentChatId]);

  // 获取当前对话
  const currentChat = chats.find(chat => chat.id === currentChatId);

  return {
    chats,
    currentChat,
    currentChatId,
    setCurrentChatId,
    createNewChat,
    updateChat,
    deleteChat,
  };
};
```

### 6.4 文件上传处理

```javascript
// utils/fileUtils.js

// 支持的文件类型
export const SUPPORTED_FILE_TYPES = {
  images: {
    types: ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/svg+xml'],
    extensions: ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.svg'],
    maxSize: 10 * 1024 * 1024, // 10MB
  },
  documents: {
    types: [
      'application/pdf',
      'application/msword',
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      'application/vnd.ms-excel',
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'text/plain',
      'text/csv',
    ],
    extensions: ['.pdf', '.doc', '.docx', '.xls', '.xlsx', '.txt', '.csv'],
    maxSize: 20 * 1024 * 1024, // 20MB
  },
};

// 验证文件类型
export const validateFileType = (file, category = 'images') => {
  const config = SUPPORTED_FILE_TYPES[category];
  if (!config) return false;
  
  const isValidType = config.types.includes(file.type);
  const isValidExtension = config.extensions.some(ext => 
    file.name.toLowerCase().endsWith(ext)
  );
  const isValidSize = file.size <= config.maxSize;
  
  return {
    valid: isValidType && isValidExtension && isValidSize,
    error: !isValidType || !isValidExtension 
      ? '不支持的文件类型' 
      : !isValidSize 
      ? `文件大小超过限制 (${(config.maxSize / 1024 / 1024).toFixed(0)}MB)`
      : null,
  };
};

// 将文件转换为 Base64
export const fileToBase64 = (file) => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
};

// 压缩图片
export const compressImage = async (file, maxWidth = 1024, quality = 0.8) => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = (e) => {
      const img = new Image();
      img.onload = () => {
        const canvas = document.createElement('canvas');
        let width = img.width;
        let height = img.height;

        if (width > maxWidth) {
          height = (height * maxWidth) / width;
          width = maxWidth;
        }

        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext('2d');
        ctx.drawImage(img, 0, 0, width, height);

        canvas.toBlob(
          (blob) => resolve(blob),
          'image/jpeg',
          quality
        );
      };
      img.onerror = reject;
      img.src = e.target.result;
    };
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
};

// 处理 PDF 文件（提取文本）
export const extractPDFText = async (file) => {
  // 需要安装 pdfjs-dist
  // npm install pdfjs-dist
  const pdfjsLib = await import('pdfjs-dist');
  pdfjsLib.GlobalWorkerOptions.workerSrc = `//cdnjs.cloudflare.com/ajax/libs/pdf.js/${pdfjsLib.version}/pdf.worker.min.js`;
  
  const arrayBuffer = await file.arrayBuffer();
  const pdf = await pdfjsLib.getDocument({ data: arrayBuffer }).promise;
  
  let fullText = '';
  for (let i = 1; i <= pdf.numPages; i++) {
    const page = await pdf.getPage(i);
    const textContent = await page.getTextContent();
    const pageText = textContent.items.map(item => item.str).join(' ');
    fullText += pageText + '\n\n';
  }
  
  return fullText;
};

// 处理 Word 文档（提取文本）
export const extractWordText = async (file) => {
  // 需要安装 mammoth
  // npm install mammoth
  const mammoth = await import('mammoth');
  const arrayBuffer = await file.arrayBuffer();
  const result = await mammoth.extractRawText({ arrayBuffer });
  return result.value;
};

// 处理文本文件
export const readTextFile = (file) => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = (e) => resolve(e.target.result);
    reader.onerror = reject;
    reader.readAsText(file);
  });
};

// 统一文件处理入口
export const processFile = async (file) => {
  const fileType = file.type;
  const fileName = file.name;
  
  try {
    // 图片文件
    if (fileType.startsWith('image/')) {
      const validation = validateFileType(file, 'images');
      if (!validation.valid) {
        throw new Error(validation.error);
      }
      
      // 压缩图片
      const compressed = await compressImage(file);
      const base64 = await fileToBase64(compressed);
      
      return {
        type: 'image',
        name: fileName,
        data: base64,
        size: file.size,
      };
    }
    
    // PDF 文件
    if (fileType === 'application/pdf') {
      const validation = validateFileType(file, 'documents');
      if (!validation.valid) {
        throw new Error(validation.error);
      }
      
      const text = await extractPDFText(file);
      
      return {
        type: 'pdf',
        name: fileName,
        text: text,
        size: file.size,
      };
    }
    
    // Word 文档
    if (fileType.includes('word') || fileName.endsWith('.docx') || fileName.endsWith('.doc')) {
      const validation = validateFileType(file, 'documents');
      if (!validation.valid) {
        throw new Error(validation.error);
      }
      
      const text = await extractWordText(file);
      
      return {
        type: 'word',
        name: fileName,
        text: text,
        size: file.size,
      };
    }
    
    // 文本文件
    if (fileType.startsWith('text/')) {
      const validation = validateFileType(file, 'documents');
      if (!validation.valid) {
        throw new Error(validation.error);
      }
      
      const text = await readTextFile(file);
      
      return {
        type: 'text',
        name: fileName,
        text: text,
        size: file.size,
      };
    }
    
    throw new Error('不支持的文件类型');
  } catch (error) {
    console.error('文件处理失败:', error);
    throw error;
  }
};

// 构建多模态消息内容
export const buildMultiModalMessage = (text, files) => {
  const content = [
    { type: 'text', text },
  ];

  files.forEach(file => {
    if (file.type === 'image') {
      content.push({
        type: 'image_url',
        image_url: {
          url: file.data,
          detail: 'auto',
        },
      });
    } else if (file.type === 'pdf' || file.type === 'word' || file.type === 'text') {
      // 将文档内容作为文本添加到消息中
      content.push({
        type: 'text',
        text: `\n\n[文件: ${file.name}]\n${file.text}`,
      });
    }
  });

  return content;
};

// 格式化文件大小
export const formatFileSize = (bytes) => {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const sizes = ['Bytes', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
};

// 获取文件图标
export const getFileIcon = (fileType) => {
  const iconMap = {
    'image': '🖼️',
    'pdf': '📄',
    'word': '📝',
    'excel': '📊',
    'text': '📃',
    'csv': '📈',
  };
  return iconMap[fileType] || '📎';
};
```

## 七、UI/UX 设计要点

### 7.1 响应式设计
- **桌面端**: 三栏布局（侧边栏 + 主区域 + 设置面板）
- **平板端**: 可折叠侧边栏，主区域 + 设置面板
- **移动端**: 单栏布局，通过抽屉展示侧边栏和设置

### 7.2 交互设计

#### 7.2.1 令牌选择交互
- **令牌下拉框**:
  - 显示令牌名称、剩余额度、状态标签
  - 额度不足时显示橙色警告标签
  - 额度用尽时显示红色标签且不可选
  - 支持搜索过滤令牌

- **模型下拉框**:
  - 根据选中的令牌动态更新
  - 显示模型名称、提供商、支持的端点类型
  - 支持搜索过滤模型
  - 未选择令牌时禁用且显示提示

- **令牌切换提示**:
  - 切换令牌时，如果当前模型不可用，自动清空模型选择
  - 显示友好的提示信息："该令牌不支持当前模型，请重新选择"

#### 7.2.2 快捷键支持
  - `Ctrl/Cmd + Enter`: 发送消息
  - `Ctrl/Cmd + N`: 新建对话
  - `Ctrl/Cmd + K`: 聚焦搜索
  - `Esc`: 停止生成

- **加载状态**:
  - 发送消息时显示加载动画
  - 流式响应时显示打字效果
  - 骨架屏加载历史记录

- **错误处理**:
  - 网络错误自动重试
  - 显示友好的错误提示
  - 支持手动重新发送

### 7.3 视觉设计

#### 7.3.1 令牌状态颜色
- **正常状态**: 蓝色标签
- **无限额度**: 绿色标签
- **余额不足**: 橙色标签（剩余额度 < 1元）
- **已用尽**: 红色标签（剩余额度 = 0）
- **已过期**: 灰色标签

#### 7.3.2 配色方案 
  - 主色: 紫色渐变 (#8B5CF6 → #3B82F6)
  - 用户消息: 浅蓝色背景
  - AI 消息: 白色/浅灰背景
  - 代码块: 深色主题

- **动画效果**:
  - 消息淡入动画
  - 流式打字效果
  - 按钮悬停效果
  - 面板展开/收起动画

## 八、性能优化

### 8.1 前端优化
- 使用 React.memo 优化组件渲染
- 虚拟滚动处理长对话列表
- 图片懒加载
- 防抖处理输入事件
- 代码分割和懒加载

### 8.2 数据优化
- 本地缓存对话历史
- 分页加载历史记录
- 压缩图片上传
- 限制消息历史长度

## 九、安全考虑

### 9.1 数据安全
- Token 加密存储
- HTTPS 传输
- XSS 防护（内容过滤）
- CSRF 防护

### 9.2 隐私保护
- 本地存储敏感数据
- 支持清除历史记录
- 不上传用户隐私信息

## 十、开发计划

### Phase 1: 基础功能（1-2周）
- [ ] 页面布局搭建
- [ ] 令牌选择功能
- [ ] 根据令牌动态加载模型
- [ ] 基础对话功能
- [ ] 消息渲染（Markdown + 代码高亮）

### Phase 2: 核心功能（2-3周）
- [ ] 流式响应
- [ ] 对话历史管理
- [ ] 图片上传与识别

### Phase 3: 高级功能（1-2周）
- [ ] 消息编辑与重新生成
- [ ] 对话导出
- [ ] Token 统计
- [ ] 快捷键支持

### Phase 4: 优化与测试（1周）
- [ ] 性能优化
- [ ] 移动端适配
- [ ] 错误处理完善
- [ ] 用户测试与反馈

## 十一、参考资源

- [OpenAI API 文档](https://platform.openai.com/docs/api-reference)
- [Semi Design 组件库](https://semi.design/)
- [React Markdown](https://github.com/remarkjs/react-markdown)
- [Highlight.js](https://highlightjs.org/)
