<script setup lang="ts">
/**
 * 统一的「加载失败」占位：给出可操作的说明与重试入口，替代一闪而过的 toast。
 *
 * 有些错误「重试」必然再失败（比如源站已被下线），这时光给重试就是死路。
 * 调用方通过 #actions 插槽补充别的出口（换源搜索、清理记录等），
 * 并可传 retryHidden 把无意义的重试按钮收起来。
 */
defineProps<{
  message?: string
  title?: string
  /** 隐藏「重新加载」——错误不可恢复时用，避免用户反复点同一个死路 */
  retryHidden?: boolean
  /** 重试按钮文案 */
  retryText?: string
}>()
const emit = defineEmits<{ (e: 'retry'): void }>()
</script>

<template>
  <div class="error-state">
    <div class="es-icon">!</div>
    <p class="es-title">{{ title || '加载失败' }}</p>
    <p class="es-sub">{{ message || '可以检查网络或站源后重试' }}</p>
    <div class="es-actions">
      <el-button v-if="!retryHidden" size="small" type="primary" @click="emit('retry')">
        {{ retryText || '重新加载' }}
      </el-button>
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 48px 20px;
  text-align: center;
  color: var(--sub);
}
.es-icon {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  border: 1px solid var(--warn);
  color: var(--warn);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  font-weight: 600;
}
.es-title { font-size: 15px; color: var(--txt); margin: 0; }
.es-sub { font-size: 13px; color: var(--dim); margin: 0 0 6px; max-width: 420px; }
/* 主/次操作横排，窄屏自动换行 */
.es-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  justify-content: center;
}
</style>
