<script setup lang="ts">
/**
 * 全局统一表单字段：label + 控件 + 提示 / 错误。
 *
 * <p>与 FormDialog 配套使用，控件统一 28px 高 / 12px 字号（见 --wb-ctl-h / --wb-fs-ctl）。
 */
interface Props {
  label?: string
  hint?: string
  error?: string
  required?: boolean
  /** 整行独占（表单为 2 列栅格时）。 */
  full?: boolean
}

withDefaults(defineProps<Props>(), {
  label: '',
  hint: '',
  error: '',
  required: false,
  full: false
})
</script>

<template>
  <div class="field" :class="{ 'field--full': full }">
    <label v-if="label">
      {{ label }}<span v-if="required" class="field__req">*</span>
    </label>
    <slot />
    <p v-if="error" class="field__msg field__msg--err">{{ error }}</p>
    <p v-else-if="hint" class="field__msg">{{ hint }}</p>
  </div>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.field--full {
  grid-column: 1 / -1;
}
.field > label {
  font-size: var(--wb-fs-label, 11px);
  font-weight: 500;
  color: var(--wb-muted);
}
.field__req {
  margin-left: 3px;
  color: var(--wb-danger);
}
.field__msg {
  margin: 0;
  font-size: var(--wb-fs-hint, 11px);
  line-height: 1.5;
  color: var(--wb-muted);
}
.field__msg--err {
  color: var(--wb-danger);
}
</style>
