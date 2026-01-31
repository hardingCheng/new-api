import React from 'react';
import { Typography, Card, Tag } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { IconTerminal, IconStar } from '@douyinfe/semi-icons';
import { useIsMobile } from '../../hooks/common/useIsMobile';

const { Title, Text } = Typography;

const PricingComparison = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();

  return (
    <section className='py-12 md:py-16 lg:py-20 px-4 md:px-6'>
      <div className='max-w-6xl mx-auto'>
        {/* 顶部标签和标题 */}
        <div className='text-center mb-10 md:mb-12 lg:mb-14'>
          <div className='inline-block mb-4 md:mb-5 animate-bounce-slow'>
            <Tag
              size='large'
              color='red'
              type='light'
              style={{ 
                borderRadius: '20px', 
                padding: isMobile ? '5px 16px' : '6px 20px',
                fontSize: isMobile ? '13px' : '14px',
                fontWeight: 600,
                boxShadow: '0 4px 12px rgba(245, 34, 45, 0.15)',
                border: '1px solid rgba(245, 34, 45, 0.2)',
              }}
            >
              🔥 {t('超低价格')}
            </Tag>
          </div>
          
          <Title 
            heading={1} 
            className='!text-3xl md:!text-4xl lg:!text-5xl !font-extrabold !mb-3 md:!mb-4 !leading-tight'
          >
            {t('官方价格的')} 
            <span 
              className='price-pulse inline-block ml-2 md:ml-3'
              style={{ 
                color: 'rgb(245, 34, 45)', 
                fontSize: isMobile ? '1.4em' : '1.6em',
                fontWeight: 900,
                textShadow: '0 4px 16px rgba(245, 34, 45, 0.4)',
              }}
            >
              1/20
            </span>
          </Title>
          
          <Text 
            className='text-base md:text-lg font-semibold'
            style={{ color: 'var(--semi-color-text-1)' }}
          >
            <span 
              className='font-bold text-lg md:text-xl'
              style={{ color: 'rgb(245, 34, 45)' }}
            >
              1 元人民币 = 1 美元额度
            </span>
            ，{t('真正的超值体验')}
          </Text>
        </div>

        {/* 三个卡片 */}
        <div className='grid grid-cols-1 md:grid-cols-3 gap-5 md:gap-6'>
          {/* Claude Code 卡片 */}
          <Card 
            bordered
            className='pricing-card'
            style={{
              borderRadius: '20px',
              overflow: 'hidden',
              transition: 'all 0.3s ease',
              cursor: 'pointer',
              border: '1px solid var(--semi-color-border)',
              background: 'var(--semi-color-bg-1)',
            }}
            bodyStyle={{ padding: 0 }}
          >
            <div style={{ padding: isMobile ? '24px 16px' : '28px 20px', textAlign: 'center' }}>
              {/* 图标容器 */}
              <div 
                className='mb-4'
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  width: isMobile ? '56px' : '64px',
                  height: isMobile ? '56px' : '64px',
                  borderRadius: '16px',
                  background: 'linear-gradient(135deg, rgba(201, 100, 66, 0.1) 0%, rgba(201, 100, 66, 0.05) 100%)',
                  border: '2px solid rgba(201, 100, 66, 0.1)',
                }}
              >
                <IconTerminal 
                  size='extra-large' 
                  style={{ color: 'rgb(201, 100, 66)', fontSize: isMobile ? '24px' : '28px' }} 
                />
              </div>
              
              <Title 
                heading={4} 
                className='!mb-3 !text-lg md:!text-xl !font-bold'
              >
                Claude Code
              </Title>
              
              <div style={{ marginBottom: isMobile ? '16px' : '20px' }}>
                <div 
                  className='text-xs md:text-sm mb-3 opacity-60'
                  style={{ 
                    textDecoration: 'line-through', 
                    color: 'var(--semi-color-text-2)',
                  }}
                >
                  {t('官方价格:')} 
                  <span className='font-semibold'> $20/月 ≈ ¥146</span>
                </div>
                <div 
                  className='price-bounce'
                  style={{ 
                    fontSize: isMobile ? '36px' : '42px', 
                    fontWeight: 900, 
                    color: 'rgb(245, 34, 45)',
                    lineHeight: 1,
                    marginBottom: '6px',
                    textShadow: '0 4px 12px rgba(245, 34, 45, 0.25)',
                    letterSpacing: '-1px',
                  }}
                >
                  ¥10
                  <span style={{ fontSize: isMobile ? '18px' : '22px', fontWeight: 700, marginLeft: '4px' }}>
                    {t('起')}
                  </span>
                </div>
                <Text type='tertiary' className='text-xs md:text-sm font-medium'>
                  {t('本站价格')}
                </Text>
              </div>
              
              <Tag 
                size='large' 
                color='green' 
                type='light'
                style={{
                  borderRadius: '10px',
                  padding: isMobile ? '5px 14px' : '6px 16px',
                  fontSize: isMobile ? '13px' : '14px',
                  fontWeight: 700,
                  background: 'linear-gradient(135deg, rgba(82, 196, 26, 0.15) 0%, rgba(82, 196, 26, 0.1) 100%)',
                  border: '1px solid rgba(82, 196, 26, 0.3)',
                }}
              >
                💰 {t('节省')} <span className='number-scale' style={{ fontSize: isMobile ? '16px' : '18px', fontWeight: 900 }}>93%+</span>
              </Tag>
            </div>
          </Card>

          {/* 超值汇率卡片 - 高亮推荐 */}
          <Card 
            bordered
            className='pricing-card-featured'
            style={{
              borderRadius: '20px',
              overflow: 'visible',
              transition: 'all 0.3s ease',
              cursor: 'pointer',
              border: '2px solid rgb(250, 173, 20)',
              boxShadow: '0 8px 32px rgba(250, 173, 20, 0.25)',
              transform: isMobile ? 'scale(1)' : 'scale(1.05)',
              position: 'relative',
              background: 'var(--semi-color-bg-1)',
            }}
            bodyStyle={{ padding: 0 }}
          >
            {/* 推荐标签 */}
            <div 
              style={{
                position: 'absolute',
                top: '-12px',
                left: '50%',
                transform: 'translateX(-50%)',
                zIndex: 10,
              }}
            >
              <div 
                style={{
                  padding: '6px 20px',
                  borderRadius: '20px',
                  background: 'linear-gradient(135deg, rgb(250, 173, 20) 0%, rgb(255, 193, 7) 100%)',
                  boxShadow: '0 4px 12px rgba(250, 173, 20, 0.4)',
                  border: '2px solid white',
                }}
              >
                <span 
                  style={{
                    fontSize: '13px',
                    fontWeight: 700,
                    color: 'white',
                    letterSpacing: '0.5px',
                  }}
                >
                  ⭐ {t('最划算')}
                </span>
              </div>
            </div>

            <div style={{ padding: isMobile ? '32px 16px 24px' : '36px 20px 28px', textAlign: 'center' }}>
              {/* 金色图标 */}
              <div 
                className='mb-4'
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  width: isMobile ? '64px' : '72px',
                  height: isMobile ? '64px' : '72px',
                  borderRadius: '18px',
                  background: 'linear-gradient(135deg, rgb(255, 214, 102) 0%, rgb(250, 173, 20) 100%)',
                  boxShadow: '0 8px 24px rgba(250, 173, 20, 0.3)',
                }}
              >
                <span style={{ fontSize: isMobile ? '32px' : '36px' }}>💰</span>
              </div>
              
              <Title 
                heading={4} 
                className='!mb-3 !text-lg md:!text-xl !font-bold'
              >
                {t('超值汇率')}
              </Title>
              
              <div style={{ marginBottom: isMobile ? '16px' : '20px' }}>
                <div 
                  className='text-xs md:text-sm mb-3'
                  style={{ color: 'var(--semi-color-text-2)' }}
                >
                  {t('实际汇率:')} 
                  <span className='font-semibold'> 1 USD ≈ 7.3 CNY</span>
                </div>
                <div 
                  style={{
                    padding: isMobile ? '12px 20px' : '16px 24px',
                    borderRadius: '16px',
                    background: 'linear-gradient(135deg, rgba(255, 214, 102, 0.2) 0%, rgba(250, 173, 20, 0.15) 100%)',
                    marginBottom: '10px',
                    boxShadow: '0 4px 16px rgba(250, 173, 20, 0.15)',
                  }}
                >
                  <div 
                    className='price-glow'
                    style={{ 
                      fontSize: isMobile ? '38px' : '46px', 
                      fontWeight: 900, 
                      background: 'linear-gradient(135deg, rgb(245, 34, 45) 0%, rgb(250, 173, 20) 100%)',
                      WebkitBackgroundClip: 'text',
                      WebkitTextFillColor: 'transparent',
                      lineHeight: 1,
                      letterSpacing: '-2px',
                    }}
                  >
                    ¥1 = $1
                  </div>
                </div>
                <Text type='tertiary' className='text-xs md:text-sm font-medium'>
                  {t('本站汇率')}
                </Text>
              </div>
              
              <Tag 
                size='large' 
                color='amber' 
                type='light'
                style={{
                  borderRadius: '10px',
                  padding: isMobile ? '5px 14px' : '6px 16px',
                  fontSize: isMobile ? '13px' : '14px',
                  fontWeight: 700,
                  background: 'linear-gradient(135deg, rgba(255, 214, 102, 0.25) 0%, rgba(250, 173, 20, 0.2) 100%)',
                  border: '1px solid rgba(250, 173, 20, 0.4)',
                }}
              >
                🎉 <span className='number-scale' style={{ fontSize: isMobile ? '16px' : '18px', fontWeight: 900 }}>7</span> {t('倍价值')}
              </Tag>
            </div>
          </Card>

          {/* 按量计费卡片 */}
          <Card 
            bordered
            className='pricing-card'
            style={{
              borderRadius: '20px',
              overflow: 'hidden',
              transition: 'all 0.3s ease',
              cursor: 'pointer',
              border: '1px solid var(--semi-color-border)',
              background: 'var(--semi-color-bg-1)',
            }}
            bodyStyle={{ padding: 0 }}
          >
            <div style={{ padding: isMobile ? '24px 16px' : '28px 20px', textAlign: 'center' }}>
              {/* 图标容器 */}
              <div 
                className='mb-4'
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  width: isMobile ? '56px' : '64px',
                  height: isMobile ? '56px' : '64px',
                  borderRadius: '16px',
                  background: 'linear-gradient(135deg, rgba(66, 133, 244, 0.1) 0%, rgba(66, 133, 244, 0.05) 100%)',
                  border: '2px solid rgba(66, 133, 244, 0.1)',
                }}
              >
                <IconStar 
                  size='extra-large' 
                  style={{ color: 'rgb(66, 133, 244)', fontSize: isMobile ? '24px' : '28px' }} 
                />
              </div>
              
              <Title 
                heading={4} 
                className='!mb-3 !text-lg md:!text-xl !font-bold'
              >
                {t('按量计费')}
              </Title>
              
              <div style={{ marginBottom: isMobile ? '16px' : '20px' }}>
                <div 
                  className='text-xs md:text-sm mb-3'
                  style={{ color: 'var(--semi-color-text-2)' }}
                >
                  {t('无月费、无订阅')}
                </div>
                <div 
                  style={{ 
                    fontSize: isMobile ? '24px' : '28px', 
                    fontWeight: 800, 
                    color: 'rgb(66, 133, 244)',
                    lineHeight: 1.3,
                    marginBottom: '10px',
                    textShadow: '0 2px 8px rgba(66, 133, 244, 0.2)',
                  }}
                >
                  {t('用多少付多少')}
                </div>
                <Text type='tertiary' className='text-xs md:text-sm font-medium'>
                  {t('透明计费，随用随付')}
                </Text>
              </div>
              
              <Tag 
                size='large' 
                color='blue' 
                type='light'
                style={{
                  borderRadius: '12px',
                  padding: isMobile ? '5px 14px' : '6px 16px',
                  fontSize: isMobile ? '13px' : '14px',
                  fontWeight: 600,
                }}
              >
                ✨ {t('灵活省钱')}
              </Tag>
            </div>
          </Card>
        </div>

        {/* 底部说明 */}
        <div className='text-center mt-8 md:mt-10'>
          <div 
            style={{
              display: 'inline-block',
              padding: isMobile ? '12px 20px' : '14px 28px',
              borderRadius: '14px',
              background: 'var(--semi-color-bg-1)',
              backdropFilter: 'blur(10px)',
              boxShadow: '0 4px 20px var(--semi-color-shadow)',
              border: '1px solid var(--semi-color-border)',
            }}
          >
            <Text 
              className='text-sm md:text-base font-medium'
              style={{ color: 'var(--semi-color-text-0)' }}
            >
              💡 {t('提示：充值')} 
              <span className='number-scale' style={{ 
                color: 'rgb(245, 34, 45)', 
                fontWeight: 800, 
                fontSize: isMobile ? '15px' : '17px',
                margin: '0 3px',
              }}>
                ¥10
              </span> 
              {t('即可体验 Claude Sonnet 约')} 
              <span className='number-scale' style={{ 
                color: 'rgb(250, 173, 20)', 
                fontWeight: 800, 
                fontSize: isMobile ? '15px' : '17px',
                margin: '0 3px',
              }}>
                100 万 tokens
              </span> 
              {t('的对话量')}
            </Text>
          </div>
        </div>
      </div>

      <style jsx>{`
        .pricing-card:hover {
          transform: translateY(-8px);
          box-shadow: 0 12px 32px var(--semi-color-shadow);
        }

        .pricing-card-featured:hover {
          transform: scale(1.08) translateY(-4px);
          box-shadow: 0 16px 48px rgba(250, 173, 20, 0.35);
        }

        @keyframes bounce-slow {
          0%, 100% {
            transform: translateY(0);
          }
          50% {
            transform: translateY(-5px);
          }
        }

        .animate-bounce-slow {
          animation: bounce-slow 2s ease-in-out infinite;
        }

        /* 价格脉冲动画 */
        @keyframes price-pulse {
          0%, 100% {
            transform: scale(1);
          }
          50% {
            transform: scale(1.05);
          }
        }

        .price-pulse {
          animation: price-pulse 2s ease-in-out infinite;
        }

        /* 价格弹跳动画 */
        @keyframes price-bounce {
          0%, 100% {
            transform: translateY(0);
          }
          50% {
            transform: translateY(-3px);
          }
        }

        .price-bounce {
          display: inline-block;
          animation: price-bounce 1.5s ease-in-out infinite;
        }

        /* 价格发光动画 */
        @keyframes price-glow {
          0%, 100% {
            filter: drop-shadow(0 0 8px rgba(250, 173, 20, 0.3));
          }
          50% {
            filter: drop-shadow(0 0 16px rgba(250, 173, 20, 0.6));
          }
        }

        .price-glow {
          animation: price-glow 2s ease-in-out infinite;
        }

        /* 数字缩放动画 */
        @keyframes number-scale {
          0%, 100% {
            transform: scale(1);
          }
          50% {
            transform: scale(1.1);
          }
        }

        .number-scale {
          display: inline-block;
          animation: number-scale 1.8s ease-in-out infinite;
        }

        @media (max-width: 768px) {
          .pricing-card-featured {
            transform: scale(1) !important;
          }
          
          .pricing-card-featured:hover {
            transform: scale(1.02) translateY(-4px) !important;
          }
        }
      `}</style>
    </section>
  );
};

export default PricingComparison;
