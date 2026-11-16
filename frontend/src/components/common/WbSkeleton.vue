<!-- 骨架屏基元：列表与卡片还没到货时先摆出形状。
     网络一慢页面就整屏空白会被当成「坏了」；形状先在，内容到了才替换。 -->
<script setup lang="ts">
withDefaults(
  defineProps<{
    /** 骨架行数：列表形态用 */
    rows?: number
    /** 卡片形态：一块大轮廓 */
    block?: boolean
  }>(),
  { rows: 4, block: false },
)
</script>

<template>
  <div class="wb-skeleton" aria-hidden="true">
    <template v-if="block">
      <div class="sk-line w-40" />
      <div class="sk-block" />
    </template>
    <template v-else>
      <div v-for="index in rows" :key="index" class="sk-row" :style="{ animationDelay: `${index * 70}ms` }">
        <div class="sk-tile" />
        <div class="sk-cols">
          <div class="sk-line" :style="{ width: `${58 - (index % 3) * 12}%` }" />
          <div class="sk-line thin" :style="{ width: `${36 + (index % 2) * 18}%` }" />
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.wb-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}

.sk-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}

.sk-tile {
  flex: none;
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-r);
}

.sk-cols {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}

.sk-line,
.sk-tile,
.sk-block {
  background: linear-gradient(90deg, var(--wb-skeleton-from) 35%, var(--wb-skeleton-to) 50%, var(--wb-skeleton-from) 65%);
  background-size: 220% 100%;
  animation: wb-sk 1.4s ease-in-out infinite;
}

.sk-line {
  height: var(--wb-fs-sm);
  border-radius: var(--wb-r-sm);
}

.sk-line.thin {
  height: var(--wb-fs-3xs);
}

/* 形状锚点：顶部一条占位线的宽度档位 */
.w-40 {
  width: 40%;
}

.sk-block {
  height: 132px;
  border-radius: var(--wb-r-lg);
}

@keyframes wb-sk {
  from {
    background-position: 120% 0;
  }
  to {
    background-position: -120% 0;
  }
}
</style>
