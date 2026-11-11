<script setup lang="ts">
// 选集抽屉：关键词过滤 + 网格快速跳集。
import { computed, ref } from 'vue'

export interface EpisodeItem {
  name?: string
  url?: string
}

const props = defineProps<{
  modelValue: boolean
  title: string
  episodes: EpisodeItem[]
  currentIdx: number
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'select', idx: number): void
}>()

const keyword = ref('')

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return props.episodes
    .map((ep, i) => ({ ep, i }))
    .filter(({ ep, i }) => {
      if (!k) return true
      const name = (ep.name || `第 ${i + 1} 集`).toLowerCase()
      return name.includes(k) || String(i + 1) === k
    })
})
</script>

<template>
  <el-dialog
    :model-value="props.modelValue"
    :title="props.title"
    width="620px"
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
  >
    <el-input v-model="keyword" size="small" clearable placeholder="输入集数或名称快速定位" />
    <div class="ep-grid">
      <button
        v-for="x in filtered"
        :key="x.i"
        class="ep-cell"
        :class="{ on: x.i === props.currentIdx }"
        @click="emit('select', x.i)"
      >{{ x.ep.name || `第 ${x.i + 1} 集` }}</button>
    </div>
    <div v-if="!filtered.length" class="ep-none">暂无匹配剧集，可以换个关键词</div>
  </el-dialog>
</template>

<style scoped>
.ep-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(92px, 1fr));
  gap: 8px;
  margin-top: 12px;
  max-height: 52vh;
  overflow-y: auto;
}
.ep-cell {
  border: 1px solid var(--border);
  background: var(--card);
  color: var(--sub);
  border-radius: var(--radius-sm);
  padding: 8px 6px;
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ep-cell:hover { color: var(--txt); border-color: var(--sub); }
.ep-cell.on { background: var(--selected); color: #000; border-color: var(--selected); font-weight: 600; }
.ep-none { text-align: center; color: var(--dim); font-size: 13px; padding: 24px; }
</style>
