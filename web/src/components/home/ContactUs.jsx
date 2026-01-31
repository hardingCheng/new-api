import React from 'react';
import { Typography, Card, Tag } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const { Title, Text } = Typography;

const ContactUs = () => {
  const { t } = useTranslation();

  const contactItems = [
    {
      title: 'QQ 交流群',
      description: '扫码加入 智链-AI 交流群',
      image: '/qqq.jpg',
      tag: '群号: 175527328',
      tagColor: 'blue',
      icon: '👥',
    },
    {
      title: '微信客服',
      description: '扫码添加微信客服',
      image: '/wx.jpg',
      tag: '一对一服务',
      tagColor: 'green',
      icon: '💬',
    },
    {
      title: 'QQ 客服',
      description: '直接添加 QQ 联系客服',
      image: '/qq.jpg',
      tag: 'QQ: 3999837829',
      tagColor: 'cyan',
      icon: '🎧',
    },
  ];

  return (
    <section 
      className='py-16 relative overflow-hidden'
      style={{
        background: 'var(--semi-color-bg-0)',
      }}
    >
      {/* 装饰性背景 */}
      <div className='absolute inset-0 overflow-hidden pointer-events-none'>
        <div 
          className='absolute rounded-full'
          style={{
            width: '300px',
            height: '300px',
            background: 'radial-gradient(circle, rgba(99, 102, 241, 0.15) 0%, transparent 70%)',
            top: '50%',
            left: '-100px',
            transform: 'translateY(-50%)',
            filter: 'blur(60px)',
            opacity: 0.5,
          }}
        />
        <div 
          className='absolute rounded-full'
          style={{
            width: '300px',
            height: '300px',
            background: 'radial-gradient(circle, rgba(16, 185, 129, 0.15) 0%, transparent 70%)',
            top: '50%',
            right: '-100px',
            transform: 'translateY(-50%)',
            filter: 'blur(60px)',
            opacity: 0.5,
          }}
        />
      </div>

      <div className='max-w-5xl mx-auto px-6 relative z-10'>
        {/* 标题部分 */}
        <div className='text-center mb-12'>
          <div className='inline-block mb-4'>
            <Tag
              size='large'
              color='violet'
              type='light'
              style={{ 
                borderRadius: '20px', 
                padding: '6px 20px',
                fontSize: '14px',
                fontWeight: 600,
                border: '1px solid rgba(139, 92, 246, 0.2)',
              }}
            >
              📞 {t('联系方式')}
            </Tag>
          </div>
          
          <Title 
            heading={2} 
            style={{ 
              marginBottom: '12px',
              fontSize: '2.25rem',
              fontWeight: 800,
            }}
          >
            {t('联系我们')}
          </Title>
          
          <Text 
            style={{ 
              color: 'var(--semi-color-text-1)', 
              fontSize: '16px',
              fontWeight: 500,
            }}
          >
            {t('加入交流群或联系客服，获取帮助与最新资讯')}
          </Text>
        </div>

        {/* 三个联系方式卡片 */}
        <div className='grid grid-cols-1 md:grid-cols-3 gap-6'>
          {contactItems.map((item, index) => (
            <Card 
              key={index}
              bordered
              className='contact-card'
              style={{
                borderRadius: '20px',
                overflow: 'hidden',
                transition: 'all 0.3s ease',
                cursor: 'pointer',
                border: '1px solid var(--semi-color-border)',
                background: 'var(--semi-color-bg-1)',
                backdropFilter: 'blur(10px)',
              }}
              bodyStyle={{ padding: '28px 24px' }}
            >
              {/* 图标标识 */}
              <div 
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  width: '48px',
                  height: '48px',
                  borderRadius: '12px',
                  background: `linear-gradient(135deg, var(--semi-color-${item.tagColor}-0) 0%, var(--semi-color-${item.tagColor}-1) 100%)`,
                  marginBottom: '16px',
                  fontSize: '24px',
                }}
              >
                {item.icon}
              </div>

              {/* 标题 */}
              <Title 
                heading={4} 
                style={{ 
                  marginBottom: '8px', 
                  fontSize: '20px', 
                  fontWeight: 700,
                }}
              >
                {t(item.title)}
              </Title>

              {/* 描述 */}
              <Text 
                style={{ 
                  color: 'var(--semi-color-text-2)',
                  fontSize: '14px',
                  display: 'block',
                  marginBottom: '20px',
                }}
              >
                {t(item.description)}
              </Text>

              {/* 二维码图片 */}
              <div 
                className='mb-4'
                style={{
                  position: 'relative',
                  width: '100%',
                  paddingTop: '100%',
                  borderRadius: '16px',
                  overflow: 'hidden',
                  boxShadow: '0 8px 24px var(--semi-color-shadow)',
                  background: 'var(--semi-color-fill-0)',
                }}
              >
                <img 
                  src={item.image}
                  alt={item.title}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    height: '100%',
                    objectFit: 'cover',
                  }}
                />
              </div>

              {/* 标签 */}
              <Tag 
                size='large' 
                color={item.tagColor}
                type='light'
                style={{
                  borderRadius: '12px',
                  padding: '8px 16px',
                  fontSize: '13px',
                  fontWeight: 600,
                  width: '100%',
                  textAlign: 'center',
                  border: `1px solid var(--semi-color-${item.tagColor}-2)`,
                }}
              >
                {t(item.tag)}
              </Tag>
            </Card>
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
              💡 {t('工作时间：周一至周日 9:00-22:00')}
            </Text>
          </div>
        </div>
      </div>

      <style jsx>{`
        .contact-card:hover {
          transform: translateY(-8px);
          box-shadow: 0 16px 40px var(--semi-color-shadow);
          border-color: var(--semi-color-primary);
        }

        @media (max-width: 768px) {
          .contact-card:hover {
            transform: translateY(-4px);
          }
        }
      `}</style>
    </section>
  );
};

export default ContactUs;
