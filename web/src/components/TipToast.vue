<script setup lang="ts">
import { useAppStore } from '@/stores/app'
const app = useAppStore()
</script>

<template>
  <transition name="tip-fade">
    <div v-if="app.tip" class="tip-toast">
      <span>{{ app.tip }}</span>
    </div>
  </transition>
</template>

<style scoped>
.tip-toast {
  position: fixed;
  left: 50%;
  bottom: 120px;
  transform: translateX(-50%);
  z-index: 3000;
  max-width: 720px;
  padding: 14px 28px;
  border-radius: 28px;
  background: rgba(0, 0, 0, 0.8);
  border: 2px solid rgba(255, 255, 255, 0.3);
  color: #fff;
  font-size: 16px;
  text-align: center;
  pointer-events: none;
}
.tip-fade-enter-active,
.tip-fade-leave-active {
  transition: opacity 0.2s;
}
.tip-fade-enter-from,
.tip-fade-leave-to {
  opacity: 0;
}
</style>
