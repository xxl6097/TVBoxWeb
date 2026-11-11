<script setup lang="ts">
/** 统一的「加载失败」占位：给出可操作的说明与重试入口，替代一闪而过的 toast。 */
defineProps<{
  message?: string
  title?: string
}>()
const emit = defineEmits<{ (e: 'retry'): void }>()
</script>

<template>
  <div class="error-state">
    <div class="es-icon">!</div>
    <p class="es-title">{{ title || '加载失败' }}</p>
    <p class="es-sub">{{ message || '可以检查网络或站源后重试' }}</p>
    <el-button size="small" type="primary" @click="emit('retry')">重新加载</el-button>
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
</style>
