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
    currentSize,
    cacheStats,

    // 更新方法
    updateField,

    // 操作方法
    handleTokenChange,
    generateImage,
    resetGeneration,

    // 历史记录方法
    deleteHistoryRecord,
    clearHistory,
    loadFromHistory,
  } = useBananaImage();

  return (
    <Layout className='h-full bg-transparent'>
      <div className='h-full flex flex-col lg:flex-row'>
        {/* 左侧：参数配置区 */}
        <div
          className={`
            ${isMobile ? 'w-full' : 'w-[420px] flex-shrink-0'}
            h-full overflow-y-auto border-r border-[var(--semi-color-border)]
            bg-[var(--semi-color-bg-0)]
          `}
        >
          <div className='p-4 md:p-6'>
            {/* 标题栏 */}
            <div className='flex items-center justify-between mb-6'>
              <div className='flex items-center gap-3'>
                <span className='text-3xl'>🍌</span>
                <Title heading={4} className='!mb-0'>
                  香蕉生图
                </Title>
              </div>
              <Button
                icon={<IconHistory />}
                theme='borderless'
                onClick={() => setShowHistory(true)}
              >
                查看历史
              </Button>
            </div>

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

            {/* 生成按钮 */}
            <GenerateSection
              onGenerate={generateImage}
              isGenerating={generationStatus === 'loading'}
              disabled={!prompt.trim() || !selectedModel || !selectedToken}
              currentSize={currentSize}
              numberOfImages={numberOfImages}
            />
          </div>
        </div>

        {/* 右侧：结果展示区 */}
        <div className='flex-1 h-full overflow-y-auto bg-[var(--semi-color-bg-1)]'>
          <div className='h-full p-4 md:p-6'>
            <ResultSection
              status={generationStatus}
              error={generationError}
              images={generatedImages}
              selectedIndex={selectedImageIndex}
              onSelectImage={(index) => updateField('selectedImageIndex', index)}
              onReset={resetGeneration}
              prompt={prompt}
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
      />
    </Layout>
  );
};

export default BananaImagePage;
