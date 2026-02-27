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

import React, { useState } from 'react';
import { Layout, Typography, Button } from '@douyinfe/semi-ui';
import { IconHistory } from '@douyinfe/semi-icons';
import { useBananaImage } from '../../hooks/banana-image';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import PromptSection from './PromptSection';
import TokenSelector from './TokenSelector';
import ModelSelector from './ModelSelector';
import ReferenceImageSection from './ReferenceImageSection';
import ParamsSection from './ParamsSection';
import GenerateSection from './GenerateSection';
import ResultSection from './ResultSection';
import HistoryModal from './HistoryModal';

const { Title } = Typography;

const BananaImagePage = () => {
  const isMobile = useIsMobile();
  const [showHistory, setShowHistory] = useState(false);

  const {
    // 状态
    prompt,
    negativePrompt,
    selectedToken,
    availableTokens,
    tokensLoading,
    selectedModel,
    availableModels,
    modelsLoading,
    resolution,
    aspectRatio,
    numberOfImages,
    referenceImages,
    generationStatus,
    generationError,
    generatedImages,
    selectedImageIndex,
    historyRecords,
    historyHasMore,
    historyTotalCount,
    historyLoading,
    currentSize,
    cacheStats,
    generationStartTime,
    retryMessage,

    // 更新方法
    updateField,
    updateFields,

    // 操作方法
    handleTokenChange,
    generateImage,
    resetGeneration,

    // 历史记录方法
    deleteHistoryRecord,
    clearHistory,
    loadFromHistory,
    loadMoreHistory,
    searchHistory,
  } = useBananaImage();

  // 模拟图片数据（用于测试）
  const simulateImage = () => {
    // 生成10张测试图片
    const mockImages = Array.from({ length: 10 }, (_, index) => ({
      id: `mock-${Date.now()}-${index}`,
      url: `https://picsum.photos/500/500?random=${Date.now()}-${index}`,
      revisedPrompt: `这是第 ${index + 1} 张模拟的测试图片，用于展示图片预览和下载功能`,
    }));

    updateFields({
      generationStatus: 'success',
      generatedImages: mockImages,
      prompt: '模拟测试图片（10张）',
    });
  };

  return (
    <Layout className='h-full bg-transparent rounded-lg shadow-lg overflow-hidden'>
      <div className='h-full flex flex-col md:flex-row'>
        {/* 左侧：参数配置区 */}
        <div
          className={`
            w-full md:w-[520px] md:flex-shrink-0
            ${isMobile ? 'max-h-[50vh]' : 'h-full'}
            border-b md:border-b-0 md:border-r border-[var(--semi-color-border)]
            bg-[var(--semi-color-bg-0)]
            flex flex-col
          `}
        >
          {/* 固定在顶部的标题栏 */}
          <div className='border-b border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3 sm:p-3 md:p-4'>
            <div className='flex items-center justify-between'>
              <div className='flex items-center gap-2 md:gap-3'>
                <span className='text-2xl md:text-3xl'>🍌</span>
                <Title heading={isMobile ? 5 : 4} className='!mb-0'>
                  香蕉生图
                </Title>
              </div>
              <div className='flex gap-2'>
                {/* <Button
                  theme='borderless'
                  size='small'
                  onClick={simulateImage}
                >
                  测试
                </Button> */}
                <Button
                  icon={<IconHistory />}
                  theme='borderless'
                  size={isMobile ? 'small' : 'default'}
                  onClick={() => setShowHistory(true)}
                >
                  {isMobile ? '历史' : '查看历史'}
                </Button>
              </div>
            </div>
          </div>

          {/* 可滚动的参数区域 */}
          <div className='flex-1 overflow-y-auto'>
            <div className='p-3 sm:p-3 md:p-4'>

              {/* 令牌选择 */}
              <TokenSelector
                selectedToken={selectedToken}
                availableTokens={availableTokens}
                loading={tokensLoading}
                onChange={handleTokenChange}
              />

              {/* 模型选择 */}
              <ModelSelector
                selectedModel={selectedModel}
                availableModels={availableModels}
                loading={modelsLoading}
                onChange={(value) => updateField('selectedModel', value)}
                disabled={!selectedToken}
              />

              {/* 提示词输入 */}
              <PromptSection
                prompt={prompt}
                negativePrompt={negativePrompt}
                onPromptChange={(value) => updateField('prompt', value)}
                onNegativePromptChange={(value) => updateField('negativePrompt', value)}
                onGenerate={generateImage}
                isGenerating={generationStatus === 'loading'}
              />

              {/* 参考图片 */}
              <ReferenceImageSection
                referenceImages={referenceImages}
                onImagesChange={(images) => updateField('referenceImages', images)}
              />

              {/* 参数配置 */}
              <ParamsSection
                resolution={resolution}
                aspectRatio={aspectRatio}
                numberOfImages={numberOfImages}
                currentSize={currentSize}
                onResolutionChange={(value) => updateField('resolution', value)}
                onAspectRatioChange={(value) => updateField('aspectRatio', value)}
                onNumberOfImagesChange={(value) => updateField('numberOfImages', value)}
              />
            </div>
          </div>

          {/* 固定在底部的生成按钮 */}
          <div className='border-t border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3 sm:p-3 md:p-4'>
            <GenerateSection
              onGenerate={generateImage}
              isGenerating={generationStatus === 'loading'}
              disabled={!prompt.trim() || !selectedModel || !selectedToken}
              currentSize={currentSize}
              numberOfImages={numberOfImages}
              prompt={prompt}
              selectedModel={selectedModel}
              selectedToken={selectedToken}
              resolution={resolution}
              aspectRatio={aspectRatio}
            />
          </div>
        </div>

        {/* 右侧：结果展示区 */}
        <div className='flex-1 h-full flex flex-col bg-[var(--semi-color-bg-1)]'>
          <div className='flex-1 p-3 sm:p-3 md:p-4 overflow-y-auto'>
            <ResultSection
              status={generationStatus}
              error={generationError}
              retryMessage={retryMessage}
              images={generatedImages}
              selectedIndex={selectedImageIndex}
              onSelectImage={(index) => updateField('selectedImageIndex', index)}
              onReset={resetGeneration}
              prompt={prompt}
              startTime={generationStartTime}
              isMobile={isMobile}
            />
          </div>
        </div>
      </div>

      {/* 历史记录弹窗 */}
      <HistoryModal
        visible={showHistory}
        records={historyRecords}
        onSelect={loadFromHistory}
        onDelete={deleteHistoryRecord}
        onClear={clearHistory}
        onClose={() => setShowHistory(false)}
        cacheStats={cacheStats}
        hasMore={historyHasMore}
        onLoadMore={loadMoreHistory}
        onSearch={searchHistory}
        totalCount={historyTotalCount}
        isLoading={historyLoading}
      />
    </Layout>
  );
};

export default BananaImagePage;
