import React from 'react';
import { Link } from 'react-router-dom';
import { Button, Input, ScrollList, ScrollItem } from '@douyinfe/semi-ui';
import {
  IconCopy,
  IconPlay,
  IconGithubLogo,
  IconFile,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

const HeroBanner = ({
  serverAddress,
  endpointItems,
  endpointIndex,
  setEndpointIndex,
  handleCopyBaseURL,
  isDemoSiteMode,
  statusState,
  docsLink,
  isMobile,
  isChinese,
}) => {
  const { t } = useTranslation();

  return (
    <div className='w-full'>
      <div className='flex items-center justify-center h-full px-4 py-5 md:py-6 lg:py-8 mt-10'>
        {/* 居中内容区 */}
        <div className='flex flex-col items-center justify-center text-center max-w-4xl mx-auto'>
          <div className='flex flex-col items-center justify-center mb-6 md:mb-8'>
            {/* AI 徽章 */}
            <div className='ai-badge mb-6 animate-pulse-slow'>
              <span className='ai-badge-dot' />
              <span className='text-xs md:text-sm font-medium'>
                {t('智能连接 · 无限可能')}
              </span>
            </div>

            <h1
              className={`text-4xl md:text-5xl lg:text-6xl xl:text-7xl font-bold text-semi-color-text-0 leading-tight ${isChinese ? 'tracking-wide md:tracking-wider' : ''} animate-fade-in-up`}
            >
              <>
                {t('智链 AI')}
                <br />
                <span className='shine-text gradient-text'>{t('AI 模型统一接入平台')}</span>
              </>
            </h1>
            <p className='text-base md:text-lg lg:text-xl text-semi-color-text-1 mt-4 md:mt-6 max-w-xl animate-fade-in-up animation-delay-200'>
              {t('更好的价格，更好的稳定性，只需要将模型基址替换为：')}
            </p>

            {/* BASE URL 与端点选择 */}
            <div className='flex flex-col md:flex-row items-center justify-center gap-4 w-full mt-4 md:mt-6 max-w-md animate-fade-in-up animation-delay-400'>
              <Input
                readonly
                value={serverAddress}
                className='flex-1 !rounded-full glass-effect'
                size={isMobile ? 'default' : 'large'}
                suffix={
                  <div className='flex items-center gap-2'>
                    <ScrollList
                      bodyHeight={32}
                      style={{ border: 'unset', boxShadow: 'unset' }}
                    >
                      <ScrollItem
                        mode='wheel'
                        cycled={true}
                        list={endpointItems}
                        selectedIndex={endpointIndex}
                        onSelect={({ index }) => setEndpointIndex(index)}
                      />
                    </ScrollList>
                    <Button
                      type='primary'
                      onClick={handleCopyBaseURL}
                      icon={<IconCopy />}
                      className='!rounded-full hover-lift'
                    />
                  </div>
                }
              />
            </div>
          </div>

          {/* 操作按钮 */}
          <div className='flex flex-row gap-4 justify-center items-center animate-fade-in-up animation-delay-600'>
            <Link to='/console'>
              <Button
                theme='solid'
                type='primary'
                size={isMobile ? 'default' : 'large'}
                className='!rounded-3xl px-8 py-2 hover-lift glow-on-hover'
                icon={<IconPlay />}
              >
                {t('获取密钥')}
              </Button>
            </Link>
            {isDemoSiteMode && statusState?.status?.version ? (
              <Button
                size={isMobile ? 'default' : 'large'}
                className='flex items-center !rounded-3xl px-6 py-2 hover-lift'
                icon={<IconGithubLogo />}
                onClick={() =>
                  window.open(
                    'https://github.com/QuantumNous/new-api',
                    '_blank',
                  )
                }
              >
                {statusState.status.version}
              </Button>
            ) : (
              docsLink && (
                <Button
                  size={isMobile ? 'default' : 'large'}
                  className='flex items-center !rounded-3xl px-6 py-2 hover-lift'
                  icon={<IconFile />}
                  onClick={() => window.open(docsLink, '_blank')}
                >
                  {t('文档')}
                </Button>
              )
            )}
          </div>

          {/* 框架兼容性图标 */}
          {/* <ProviderIcons /> */}
        </div>
      </div>
    </div>
  );
};

export default HeroBanner;
