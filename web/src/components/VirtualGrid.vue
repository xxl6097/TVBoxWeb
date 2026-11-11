<script setup lang="ts">
/**
 * 轻量虚拟网格：只渲染可视区（+ overscan）的单元格，避免上千条频道一次性挂载 DOM。
 * 零依赖，按固定行高 + 列宽计算，够用于频道/收藏这类等高卡片列表。
 */
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    items: any[]
    itemHeight: number
    itemMinWidth?: number
    gap?: number
    overscan?: number
    maxHeight?: string
  }>(),
  {
    itemMinWidth: 150,
    gap: 8,
    overscan: 3,
    maxHeight: 'calc(100vh - 220px)'
  }
)

const viewport = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const width = ref(600)
const viewH = ref(500)
const cols = ref(1)
let ro: ResizeObserver | null = null

const rowH = computed(() => props.itemHeight + props.gap)
const cellW = computed(() => (width.value - (cols.value - 1) * props.gap) / Math.max(1, cols.value))
const totalRows = computed(() => Math.ceil(props.items.length / Math.max(1, cols.value)))
const startRow = computed(() => Math.max(0, Math.floor(scrollTop.value / rowH.value) - props.overscan))
const endRow = computed(() =>
  Math.min(totalRows.value, Math.ceil((scrollTop.value + viewH.value) / rowH.value) + props.overscan)
)

interface Cell {
  item: any
  index: number
  top: number
  left: number
}

const visible = computed<Cell[]>(() => {
  const out: Cell[] = []
  for (let row = startRow.value; row < endRow.value; row++) {
    for (let col = 0; col < cols.value; col++) {
      const index = row * cols.value + col
      if (index >= props.items.length) break
      out.push({
        item: props.items[index],
        index,
        top: row * rowH.value,
        left: col * (cellW.value + props.gap)
      })
    }
  }
  return out
})

function measure() {
  const el = viewport.value
  if (!el) return
  width.value = el.clientWidth
  viewH.value = el.clientHeight
  cols.value = Math.max(1, Math.floor((width.value + props.gap) / (props.itemMinWidth + props.gap)))
}

function onScroll() {
  scrollTop.value = viewport.value?.scrollTop || 0
}

onMounted(() => {
  measure()
  if (typeof ResizeObserver !== 'undefined' && viewport.value) {
    ro = new ResizeObserver(measure)
    ro.observe(viewport.value)
  }
  viewport.value?.addEventListener('scroll', onScroll, { passive: true })
})

onUnmounted(() => {
  ro?.disconnect()
  viewport.value?.removeEventListener('scroll', onScroll)
})

// 列表切换（搜索/分组）时回到顶部，避免停在新列表的中间位置
watch(
  () => props.items,
  () => {
    if (viewport.value) viewport.value.scrollTop = 0
    scrollTop.value = 0
  }
)
</script>

<template>
  <div ref="viewport" class="vg-viewport" :style="{ maxHeight }">
    <div class="vg-spacer" :style="{ height: totalRows * rowH + 'px' }">
      <div
        v-for="v in visible"
        :key="v.index"
        class="vg-cell"
        :style="{ top: v.top + 'px', left: v.left + 'px', width: cellW + 'px', height: itemHeight + 'px' }"
      >
        <slot :item="v.item" :index="v.index" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.vg-viewport {
  position: relative;
  width: 100%;
  overflow-y: auto;
  overflow-x: hidden;
}
.vg-spacer {
  position: relative;
  width: 100%;
}
.vg-cell {
  position: absolute;
  box-sizing: border-box;
}
.vg-cell > * {
  height: 100%;
  box-sizing: border-box;
}
</style>
