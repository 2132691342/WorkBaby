<script setup lang="ts">
// 手动命令：给不想（或不能）由界面代跑的人。
// 三条命令（装 / 升 / 卸）「有预览才给」：拿不到预览就是认不出渠道，
// 那时只给能确定的那条，不猜一个让用户复制去跑。
import { useI18n } from 'vue-i18n'
import type { AppRow } from '@/types/api'

const props = defineProps<{
  row: AppRow
  /** 刚复制成功的那段文本（用于把按钮文字改成「已复制」） */
  copied: string
}>()

const emit = defineEmits<{ copy: [text: string] }>()

const { t } = useI18n()
</script>

<template>
  <div class="manual">
    <div class="manual-head">{{ t('apps.manualCommands') }}</div>

    <div class="manual-row">
      <span class="manual-label">{{ t('common.install') }}</span>
      <code class="manual-cmd wb-mono selectable">{{ props.row.install_command }}</code>
      <button class="wb-btn text sm" @click="emit('copy', props.row.install_command)">
        {{ props.copied === props.row.install_command ? t('settings.copied') : t('settings.copyCommand') }}
      </button>
    </div>

    <div v-if="props.row.update_preview" class="manual-row">
      <span class="manual-label">{{ t('common.update') }}</span>
      <code class="manual-cmd wb-mono selectable">{{ props.row.update_preview }}</code>
      <button class="wb-btn text sm" @click="emit('copy', props.row.update_preview ?? '')">
        {{ props.copied === props.row.update_preview ? t('settings.copied') : t('settings.copyCommand') }}
      </button>
    </div>

    <div v-if="props.row.uninstall_preview" class="manual-row">
      <span class="manual-label">{{ t('common.uninstall') }}</span>
      <code class="manual-cmd wb-mono selectable">{{ props.row.uninstall_preview }}</code>
      <button class="wb-btn text sm" @click="emit('copy', props.row.uninstall_preview ?? '')">
        {{ props.copied === props.row.uninstall_preview ? t('settings.copied') : t('settings.copyCommand') }}
      </button>
    </div>

    <p class="manual-hint">{{ t('apps.manualHint') }}</p>
  </div>
</template>

<style scoped>
.manual {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  background: var(--wb-window);
}

.manual-head {
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  letter-spacing: var(--wb-ls-plate);
  text-transform: uppercase;
  color: var(--wb-faint);
}

.manual-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}

.manual-label {
  flex: none;
  width: 56px;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
}

.manual-cmd {
  flex: 1;
  min-width: 0;
  padding: 2px 6px;
  border-radius: var(--wb-r-xs);
  background: var(--wb-code-bg);
  border: 1px solid var(--wb-line);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-5);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.manual-hint {
  margin: 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}
</style>
