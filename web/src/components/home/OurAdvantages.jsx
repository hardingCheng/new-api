import React from 'react';
import { Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  IconCheckCircleStroked,
  IconTick,
  IconBolt,
  IconExpand,
  IconClock,
} from '@douyinfe/semi-icons';

const { Title, Text } = Typography;

const OurAdvantages = () => {
  const { t } = useTranslation();

  const advantages = [
    {
      icon: <IconBolt />,
      title: t('稳定快速'),
      description: t(
        '基于高质量的设计，确保API能够高质量运营，全球多机房部署，为您提供极致的响应速度',
      ),
      color: 'purple',
    },
    {
      icon: <IconTick />,
      title: t('低价费率'),
      description: t(
        '相对于市面的费率，我们的价格至少优惠30%及以上，帮助您节省成本',
      ),
      color: 'blue',
    },
    {
      icon: <IconCheckCircleStroked />,
      title: t('高效集成'),
      description: t('提供完整的文档和SDK，支持快速和便捷地进行对接开发'),
      color: 'green',
    },
    {
      icon: <IconExpand />,
      title: t('灵活扩展'),
      description: t('API设计支持扩展模式，满足从初创到企业级的需求'),
      color: 'cyan',
    },
    {
      icon: <IconClock />,
      title: t('实时监控'),
      description: t('提供完善的API使用监控和报表，确保服务质量'),
      color: 'amber',
    },
  ];

  return (
    <section className='w-full py-12 md:py-16 lg:py-20 px-4 md:px-6 relative overflow-hidden'>
      {/* 背景装饰 */}
      <div className='absolute inset-0 overflow-hidden pointer-events-none'>
        <div 
          className='absolute rounded-full'
          style={{
            width: '400px',
            height: '400px',
            background: 'radial-gradient(circle, rgba(168, 85, 247, 0.2) 0%, transparent 70%)',
            top: '10%',
            right: '-100px',
            filter: 'blur(80px)',
            opacity: 0.5,
          }}
        />
        <div 
          className='absolute rounded-full'
          style={{
            width: '350px',
            height: '350px',
            background: 'radial-gradient(circle, rgba(59, 130, 246, 0.2) 0%, transparent 70%)',
            bottom: '10%',
            left: '-80px',
            filter: 'blur(80px)',
            opacity: 0.5,
          }}
        />
      </div>

      <div className='max-w-7xl mx-auto relative z-10'>
        {/* 标题部分 */}
        <div className='text-center mb-12 md:mb-16 lg:mb-20'>
          <Title
            heading={2}
            className='!text-3xl md:!text-4xl lg:!text-5xl font-bold mb-4 animate-fade-in-up'
          >
            {t('我们的优势')}
          </Title>
          <Text 
            type='secondary' 
            className='text-base md:text-lg animate-fade-in-up animation-delay-200'
          >
            {t('为您提供稳定、高效、经济的API服务')}
          </Text>
        </div>

        {/* 内容区域 */}
        <div className='flex flex-col lg:flex-row items-center gap-8 lg:gap-16'>
          {/* 左侧图片 */}
          <div className='w-full lg:w-2/5 flex justify-center lg:justify-end order-2 lg:order-1'>
            <div className='relative w-full max-w-sm md:max-w-md lg:max-w-lg'>
              <img
                src='/home_me_like.png'
                alt='Our Advantages'
                className='w-full h-auto object-contain animate-float'
                loading='lazy'
              />
            </div>
          </div>

          {/* 右侧优势列表 */}
          <div className='w-full lg:w-3/5 order-1 lg:order-2'>
            <div className='space-y-3 md:space-y-4'>
              {advantages.map((advantage, index) => (
                <div
                  key={index}
                  className='flex items-start gap-3 md:gap-4 p-4 md:p-5 rounded-2xl hover:bg-semi-color-fill-0 transition-all duration-300 hover-lift animate-fade-in-up group'
                  style={{ animationDelay: `${index * 0.1}s` }}
                >
                  {/* 图标容器 */}
                  <div 
                    className='flex-shrink-0 w-12 h-12 md:w-14 md:h-14 flex items-center justify-center rounded-xl transition-all duration-300 group-hover:scale-110'
                    style={{
                      background: `linear-gradient(135deg, var(--semi-color-${advantage.color}-0) 0%, var(--semi-color-${advantage.color}-1) 100%)`,
                    }}
                  >
                    <div 
                      className='text-2xl md:text-3xl'
                      style={{ color: `var(--semi-color-${advantage.color})` }}
                    >
                      {advantage.icon}
                    </div>
                  </div>

                  {/* 文字内容 */}
                  <div className='flex-1 min-w-0'>
                    <Title 
                      heading={5} 
                      className='!text-lg md:!text-xl lg:!text-2xl mb-1 md:mb-2'
                    >
                      {advantage.title}
                    </Title>
                    <Text 
                      type='secondary' 
                      className='text-sm md:text-base leading-relaxed'
                    >
                      {advantage.description}
                    </Text>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default OurAdvantages;
