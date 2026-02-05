# BananaImage 页面规划文档

## 一、功能概述
BananaImage 是一个图像生成页面，支持用户通过 Gemini 图像模型生成图片。

## 二、支持的模型
仅支持以下 3 个 Gemini 图像模型：
- `gemini-2.5-flash-image-preview`
- `gemini-3-pro-image-preview`
- `gemini-2.5-flash-image`

## 三、令牌管理
- 从 `/api/token/` 接口获取最新的令牌列表
- 用户可以选择要使用的令牌
- 令牌选择器需要实时刷新功能

## 四、接口调用（Gemini 格式）
根据 `docs/openapi/relay.json` 中的定义，使用 Gemini 格式的接口：

### 接口地址
```
POST /v1beta/models/{model}:generateContent
```

### 请求参数
```json
{
  "contents": [
    {
      "parts": [
        {
          "text": "string"  // 图像描述提示词
        }
      ]
    }
  ],
  "generationConfig": {
    "temperature": 1.0,
    "topP": 0.95,
    "topK": 40,
    "maxOutputTokens": 8192,
    "responseMimeType": "image/jpeg"  // 或 image/png
  }
}
```

### 响应格式
```json
{
  "candidates": [
    {
      "content": {
        "parts": [
          {
            "inlineData": {
              "mimeType": "image/jpeg",
              "data": "base64_encoded_image_data"
            }
          }
        ]
      }
    }
  ]
}
```

## 五、页面功能设计

### 5.1 令牌选择区域
- 下拉选择器，显示令牌名称和状态
- 刷新按钮，重新获取令牌列表
- 显示当前选中令牌的额度信息

### 5.2 模型选择区域
- 单选按钮组，显示 3 个支持的模型
- 显示模型描述信息

### 5.3 参数配置区域
- **提示词输入框**：多行文本框，支持中英文
- **图片数量**：数字输入框，范围 1-4（Gemini 限制）
- **图片格式**：下拉选择
  - image/jpeg（默认）
  - image/png
- **生成参数**（可选高级设置）：
  - Temperature：0.0-2.0，默认 1.0
  - Top P：0.0-1.0，默认 0.95
  - Top K：1-100，默认 40

### 5.4 生成按钮
- 提交生成请求
- 显示加载状态
- 错误提示

### 5.5 结果展示区域
- 图片预览（网格布局）
- 下载按钮（单张或批量）
- 生成历史记录（可选）

## 六、技术实现要点

### 6.1 状态管理
```javascript
const [selectedToken, setSelectedToken] = useState(null);
const [selectedModel, setSelectedModel] = useState('gemini-2.5-flash-image-preview');
const [prompt, setPrompt] = useState('');
const [imageCount, setImageCount] = useState(1);
const [mimeType, setMimeType] = useState('image/jpeg');
const [temperature, setTemperature] = useState(1.0);
const [topP, setTopP] = useState(0.95);
const [topK, setTopK] = useState(40);
const [generatedImages, setGeneratedImages] = useState([]);
const [loading, setLoading] = useState(false);
```

### 6.2 API 调用
```javascript
// 获取令牌列表
const fetchTokens = async () => {
  const response = await API.get('/api/token/');
  return response.data;
};

// 生成图片（Gemini 格式）
const generateImage = async () => {
  const requests = [];

  // 根据图片数量创建多个请求
  for (let i = 0; i < imageCount; i++) {
    requests.push(
      API.post(`/v1beta/models/${selectedModel}:generateContent`, {
        contents: [
          {
            parts: [
              {
                text: prompt
              }
            ]
          }
        ],
        generationConfig: {
          temperature,
          topP,
          topK,
          maxOutputTokens: 8192,
          responseMimeType: mimeType
        }
      }, {
        headers: {
          'Authorization': `Bearer ${selectedToken.key}`
        }
      })
    );
  }

  const responses = await Promise.all(requests);
  return responses.map(res => res.data);
};
```

### 6.3 图片处理
```javascript
// 从响应中提取 base64 图片数据
const extractImages = (responses) => {
  return responses.map(response => {
    const candidate = response.candidates[0];
    const part = candidate.content.parts[0];
    const { mimeType, data } = part.inlineData;
    return {
      mimeType,
      base64: data,
      dataUrl: `data:${mimeType};base64,${data}`
    };
  });
};

// 下载图片
const downloadImage = (image, index) => {
  const link = document.createElement('a');
  link.href = image.dataUrl;
  link.download = `generated-image-${index + 1}.${image.mimeType.split('/')[1]}`;
  link.click();
};
```

### 6.4 错误处理
- 令牌无效或过期
- 额度不足
- 网络请求失败
- 参数验证失败
- 内容安全过滤

### 6.5 UI 组件库
使用项目现有的 UI 组件库（如 Semantic UI React）保持风格一致。

## 七、开发步骤
1. 创建页面组件结构
2. 实现令牌选择功能
3. 实现模型选择和参数配置
4. 实现图片生成 API 调用（Gemini 格式）
5. 实现结果展示和下载功能
6. 添加错误处理和加载状态
7. 样式优化和响应式适配
8. 测试和调试

## 八、注意事项
1. Gemini 图像模型使用 `generateContent` 接口，返回 base64 格式图片
2. 需要为每张图片发送单独的请求（并发处理）
3. 图片数据较大，注意内存管理和加载状态提示
4. 支持 JPEG 和 PNG 两种格式
5. 需要处理内容安全过滤的情况
