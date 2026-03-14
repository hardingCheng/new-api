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

import React from 'react';
import { Modal } from '@douyinfe/semi-ui';

const PinUserModal = ({ visible, onCancel, onConfirm, user, action, t }) => {
  if (!user) return null;

  const isPin = action === 'pin';
  const title = isPin ? t('置顶用户') : t('取消置顶用户');
  const content = isPin
    ? t('确定要置顶用户 {{username}} 吗？置顶后该用户将显示在列表顶部。', {
        username: user.username,
      })
    : t('确定要取消置顶用户 {{username}} 吗？', { username: user.username });

  return (
    <Modal
      title={title}
      visible={visible}
      onOk={onConfirm}
      onCancel={onCancel}
      centered
      size='small'
      okText={t('确定')}
      cancelText={t('取消')}
    >
      <p>{content}</p>
    </Modal>
  );
};

export default PinUserModal;
