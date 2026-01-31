import React from 'react';
import { Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  IconCheckCircleStroked,
  IconTick,
  IconStar,
  IconBolt,
  IconExpand,
  IconClock,
} from '@douyinfe/semi-icons';

const { Title, Text } = Typography;

const OurAdvantages = () => {
  const { t } = useTranslation();

  const advantages = [
    {
      icon: <IconBolt className='text-purple-500' />,
      title: t('稳定快速'),
      description: t(
        '基于高质量的设计，确保API能够高质量运营，全球多机房部署，为您提供极致的响应速度',
      ),
    },
    {
      icon: <IconTick className='text-blue-500' />,
      title: t('低价费率'),
      description: t(
        '相对于市面的费率，我们的价格至少优惠30%及以上，帮助您节省成本',
      ),
    },
    {
      icon: <IconCheckCircleStroked className='text-green-500' />,
      title: t('高效集成'),
      description: t('提供完整的文档和SDK，支持快速和便捷地进行对接开发'),
    },
    {
      icon: <IconExpand className='text-cyan-500' />,
      title: t('灵活扩展'),
      description: t('API设计支持扩展模式，满足从初创到企业级的需求'),
    },
    {
      icon: <IconClock className='text-yellow-500' />,
      title: t('实时监控'),
      description: t('提供完善的API使用监控和报表，确保服务质量'),
    },
  ];

  return (
    <div className='w-full py-8 md:py-10 lg:py-12 px-4'>
      <div className='max-w-7xl mx-auto'>
        {/* 标题 */}
        <div className='text-center mb-12 md:mb-16'>
          <Title
            heading={2}
            className='!text-3xl md:!text-4xl lg:!text-5xl font-bold mb-4'
          >
            {t('我们的优势')}
          </Title>
        </div>

        {/* 内容区域 */}
        <div className='flex flex-col lg:flex-row items-center gap-8 lg:gap-12'>
          {/* 左侧图片 */}
          <div className='w-full lg:w-1/2 flex justify-center lg:justify-end'>
            <div className='relative w-full max-w-md lg:max-w-lg'>
              <img
                src='/home_me_like.png'
                alt='Our Advantages'
                className='w-full h-auto object-contain animate-float'
              />
            </div>
          </div>

          {/* 右侧优势列表 */}
          <div className='w-full lg:w-1/2'>
            {advantages.map((advantage, index) => (
              <div
                key={index}
                className='flex items-start gap-4 p-4 rounded-2xl hover:bg-semi-color-fill-0 transition-all duration-300 hover-lift animate-fade-in-up'
                style={{ animationDelay: `${index * 0.1}s` }}
              >
                {/* 图标 */}
                <div className='flex-shrink-0 w-10 h-10 flex items-center justify-center rounded-xl bg-semi-color-fill-1'>
                  <div className='text-2xl'>{advantage.icon}</div>
                </div>

                {/* 文字内容 */}
                <div className='flex-1 min-w-0'>
                  <Title heading={5} className='!text-lg md:!text-xl mb-2'>
                    {advantage.title}
                  </Title>
                  <Text type='secondary' className='text-sm md:text-base'>
                    {advantage.description}
                  </Text>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};

export default OurAdvantages;
