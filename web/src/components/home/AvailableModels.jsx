import React, { useState } from 'react';
import { Typography, Card, Tag, Tabs, TabPane, Toast } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { Claude, OpenAI, Gemini } from '@lobehub/icons';
import { IconCopy } from '@douyinfe/semi-icons';

const { Title, Text } = Typography;

const AvailableModels = () => {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState('claude');

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text).then(() => {
      Toast.success(t('已复制到剪贴板'));
    }).catch(() => {
      Toast.error(t('复制失败'));
    });
  };

  const modelProviders = [
    {
      key: 'claude',
      name: 'Claude',
      icon: <Claude.Color size={20} />,
      color: 'orange',
      models: [
        {
          name: 'claude-opus-4-5-20251101',
          description: '最强推理',
          type: '旗舰',
          typeColor: 'orange',
        },
        {
          name: 'claude-opus-4-5-20251101-thinking',
          description: '深度思考',
          type: '推理',
          typeColor: 'purple',
        },
        {
          name: 'claude-opus-4-20250514',
          description: '旗舰版本',
          type: '旗舰',
          typeColor: 'orange',
        },
        {
          name: 'claude-opus-4-20250514-thinking',
          description: '旗舰思考版',
          type: '推理',
          typeColor: 'purple',
        },
        {
          name: 'claude-opus-4-1-20250805',
          description: '优化版本',
          type: '旗舰',
          typeColor: 'orange',
        },
        {
          name: 'claude-opus-4-1-20250805-thinking',
          description: '优化思考版',
          type: '推理',
          typeColor: 'purple',
        },
        {
          name: 'claude-sonnet-4-5-20250929',
          description: '性价比之选',
          type: '推荐',
          typeColor: 'green',
        },
        {
          name: 'claude-sonnet-4-5-20250929-thinking',
          description: '思考版',
          type: '推理',
          typeColor: 'purple',
        },
        {
          name: 'claude-sonnet-4-20250514',
          description: '均衡版本',
          type: '推荐',
          typeColor: 'green',
        },
        {
          name: 'claude-haiku-4-5-20251001',
          description: '快速响应',
          type: '快速',
          typeColor: 'blue',
        },
        {
          name: 'claude-haiku-4-5-20251001-thinking',
          description: '快速思考版',
          type: '推理',
          typeColor: 'purple',
        },
      ],
    },
    {
      key: 'openai',
      name: 'OpenAI',
      icon: <OpenAI size={20} type='color' />,
      color: 'green',
      models: [
        {
          name: 'gpt-5',
          description: '最新旗舰',
          type: '旗舰',
          typeColor: 'orange',
        },
        {
          name: 'gpt-5-codex',
          description: '代码专用',
          type: '代码',
          typeColor: 'purple',
        },
        {
          name: 'gpt-5.1',
          description: '增强版本',
          type: '旗舰',
          typeColor: 'orange',
        },
        {
          name: 'gpt-5.1-codex',
          description: '代码增强版',
          type: '代码',
          typeColor: 'purple',
        },
        {
          name: 'gpt-5.2',
          description: '企业级推理',
          type: '旗舰',
          typeColor: 'orange',
        },
      ],
    },
    {
      key: 'gemini',
      name: 'Gemini',
      icon: <Gemini.Color size={20} type='color' />,
      color: 'blue',
      models: [
        {
          name: 'gemini-2.5-pro',
          description: '高性能版',
          type: '旗舰',
          typeColor: 'orange',
        },
        {
          name: 'gemini-3-flash-preview',
          description: '快速预览版',
          type: '快速',
          typeColor: 'blue',
        },
        {
          name: 'gemini-3-flash-preview-thinking',
          description: '快速思考版',
          type: '推理',
          typeColor: 'purple',
        },
        {
          name: 'gemini-3-pro-preview',
          description: '专业预览版',
          type: '旗舰',
          typeColor: 'orange',
        },
        {
          name: 'gemini-3-pro-preview-thinking',
          description: '专业思考版',
          type: '推理',
          typeColor: 'purple',
        },
      ],
    },
  ];

  return (
    <section className='w-full py-12 md:py-16 lg:py-20 px-4 md:px-6'>
      <div className='max-w-6xl mx-auto'>
        {/* 标题部分 */}
        <div className='text-center mb-12 md:mb-16'>
          <Title
            heading={2}
            className='!text-3xl md:!text-4xl lg:!text-5xl font-bold mb-4 animate-fade-in-up'
          >
            {t('可用模型')}
          </Title>
          <Text 
            type='secondary' 
            className='text-base md:text-lg animate-fade-in-up animation-delay-200'
          >
            {t('Claude 全系列 + Gemini，持续更新中')}
          </Text>
        </div>

        {/* Tabs 切换 */}
        <Tabs
          type='button'
          activeKey={activeTab}
          onChange={setActiveTab}
          className='mb-8'
          style={{
            display: 'flex',
            justifyContent: 'center',
          }}
          tabBarStyle={{
            display: 'flex',
            justifyContent: 'center',
            gap: '12px',
            borderBottom: 'none',
          }}
        >
          {modelProviders.map((provider) => (
            <TabPane
              key={provider.key}
              tab={
                <div className='flex items-center gap-2 px-4 py-2'>
                  {provider.icon}
                  <span className='font-semibold'>{provider.name}</span>
                </div>
              }
              itemKey={provider.key}
            />
          ))}
        </Tabs>

        {/* 模型列表 */}
        <div className='animate-fade-in-up'>
          {modelProviders.map((provider) => (
            activeTab === provider.key && (
              <div key={provider.key}>
                {/* 表头 */}
                <div className='grid grid-cols-3 gap-4 px-6 py-4 mb-2 rounded-xl bg-semi-color-fill-0'>
                  <Text strong className='text-sm md:text-base'>
                    {t('模型')}
                  </Text>
                  <Text strong className='text-sm md:text-base text-center'>
                    {t('说明')}
                  </Text>
                  <Text strong className='text-sm md:text-base text-right'>
                    {t('类型')}
                  </Text>
                </div>

                {/* 模型行 */}
                <div className='space-y-2'>
                  {provider.models.map((model, index) => (
                    <Card
                      key={index}
                      bordered
                      className='hover-lift'
                      style={{
                        borderRadius: '12px',
                        transition: 'all 0.3s ease',
                        cursor: 'pointer',
                        border: '1px solid var(--semi-color-border)',
                      }}
                      bodyStyle={{ padding: '20px 24px' }}
                    >
                      <div className='grid grid-cols-3 gap-4 items-center'>
                        {/* 模型名称 */}
                        <div className='flex items-center gap-3'>
                          <Text
                            strong
                            className='text-sm md:text-base font-mono'
                            style={{ wordBreak: 'break-all' }}
                          >
                            {model.name}
                          </Text>
                          <IconCopy
                            size='default'
                            style={{ 
                              cursor: 'pointer',
                              color: 'var(--semi-color-text-2)',
                              flexShrink: 0,
                              fontSize: '18px'
                            }}
                            onClick={(e) => {
                              e.stopPropagation();
                              copyToClipboard(model.name);
                            }}
                            onMouseEnter={(e) => {
                              e.currentTarget.style.color = 'var(--semi-color-primary)';
                            }}
                            onMouseLeave={(e) => {
                              e.currentTarget.style.color = 'var(--semi-color-text-2)';
                            }}
                          />
                        </div>

                        {/* 说明 */}
                        <div className='text-center'>
                          <Text
                            type='secondary'
                            className='text-sm md:text-base'
                          >
                            {t(model.description)}
                          </Text>
                        </div>

                        {/* 类型标签 */}
                        <div className='flex justify-end'>
                          <Tag
                            color={model.typeColor}
                            size='large'
                            style={{
                              borderRadius: '8px',
                              padding: '4px 12px',
                              fontWeight: 600,
                            }}
                          >
                            {t(model.type)}
                          </Tag>
                        </div>
                      </div>
                    </Card>
                  ))}
                </div>
              </div>
            )
          ))}
        </div>

        {/* 底部提示 */}
        <div className='text-center mt-10'>
          <div 
            style={{
              display: 'inline-block',
              padding: '12px 24px',
              borderRadius: '12px',
              background: 'var(--semi-color-bg-1)',
              backdropFilter: 'blur(10px)',
              boxShadow: '0 4px 16px var(--semi-color-shadow)',
              border: '1px solid var(--semi-color-border)',
            }}
          >
            <Text style={{ fontSize: '14px', color: 'var(--semi-color-text-1)', fontWeight: 500 }}>
              💡 {t('更多模型持续接入中，敬请期待')}
            </Text>
          </div>
        </div>
      </div>

      <style jsx>{`
        .hover-lift:hover {
          transform: translateY(-2px);
          box-shadow: 0 8px 24px var(--semi-color-shadow);
          border-color: var(--semi-color-primary) !important;
        }

        @media (max-width: 768px) {
          .hover-lift:hover {
            transform: translateY(-1px);
          }
        }
      `}</style>
    </section>
  );
};

export default AvailableModels;
