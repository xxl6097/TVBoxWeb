<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { api } from '@/api'
import TopBar from '@/components/TopBar.vue'

interface MetricsData {
  uptime: number
  requests: { total: number; errors: number; avgMs: number; p50Ms: number; p95Ms: number }
  cache: { hits: number; hitRate: number }
  memory: { allocMb: number; totalAllocMb: number; sysMb: number; heapObjects: number; gcCycles: number }
  runtime: { goroutines: number; cpus: number }
  websocket: { connections: number }
  topPaths: { path: string; count: number }[]
}

const metrics = ref<MetricsData | null>(null)
const loading = ref(true)
const autoRefresh = ref(true)
let timer = 0

onMounted(async () => {
  await refresh()
  // 定时刷新（3s）
  timer = window.setInterval(() => {
    if (autoRefresh.value) refresh()
  }, 3000)
})

onUnmounted(() => clearInterval(timer))

async function refresh() {
  try {
    metrics.value = await api.metrics()
    loading.value = false
  } catch {
    /* 忽略错误，保留上次数据 */
  }
}

// 格式化 uptime
const uptimeText = computed(() => {
  if (!metrics.value) return ''
  const s = metrics.value.uptime
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d > 0) return `${d} 天 ${h} 小时`
  if (h > 0) return `${h} 小时 ${m} 分`
  return `${m} 分 ${s % 60} 秒`
})

// 缓存命中率百分比
const cacheRateText = computed(() => {
  if (!metrics.value) return '0%'
  return (metrics.value.cache.hitRate * 100).toFixed(1) + '%'
})

// 最大 top path count（用于条形图比例）
const maxPathCount = computed(() => {
  if (!metrics.value || !metrics.value.topPaths.length) return 1
  return metrics.value.topPaths[0].count
})
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="metrics-head">
      <div class="metrics-title">性能监控</div>
      <div class="metrics-actions">
        <el-switch v-model="autoRefresh" active-text="自动刷新" size="small" />
        <el-button size="small" @click="refresh">立即刷新</el-button>
      </div>
    </div>

    <div v-if="loading && !metrics" class="metrics-loading">
      <span>加载监控数据…</span>
    </div>

    <template v-else-if="metrics">
      <!-- 概览卡片 -->
      <div class="metrics-grid">
        <div class="m-card">
          <div class="m-label">运行时长</div>
          <div class="m-value">{{ uptimeText }}</div>
          <div class="m-sub">服务已稳定运行</div>
        </div>
        <div class="m-card">
          <div class="m-label">API 请求总数</div>
          <div class="m-value">{{ metrics.requests.total }}</div>
          <div class="m-sub">错误 {{ metrics.requests.errors }} 次</div>
        </div>
        <div class="m-card">
          <div class="m-label">缓存命中率</div>
          <div class="m-value">{{ cacheRateText }}</div>
          <div class="m-sub">命中 {{ metrics.cache.hits }} 次（ETag 304）</div>
        </div>
        <div class="m-card">
          <div class="m-label">WebSocket 连接</div>
          <div class="m-value">{{ metrics.websocket.connections }}</div>
          <div class="m-sub">实时在线客户端</div>
        </div>
      </div>

      <!-- 响应时间 -->
      <div class="metrics-section">
        <div class="section-title">响应时间</div>
        <div class="m-card m-card-wide">
          <div class="latency-row">
            <div class="latency-item">
              <div class="m-label">平均</div>
              <div class="m-value small">{{ metrics.requests.avgMs }}<span class="unit">ms</span></div>
            </div>
            <div class="latency-item">
              <div class="m-label">P50</div>
              <div class="m-value small">{{ metrics.requests.p50Ms }}<span class="unit">ms</span></div>
            </div>
            <div class="latency-item">
              <div class="m-label">P95</div>
              <div class="m-value small">{{ metrics.requests.p95Ms }}<span class="unit">ms</span></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 内存与运行时 -->
      <div class="metrics-section">
        <div class="section-title">资源占用</div>
        <div class="metrics-grid two">
          <div class="m-card">
            <div class="m-label">当前内存</div>
            <div class="m-value">{{ metrics.memory.allocMb }}<span class="unit">MB</span></div>
            <div class="m-sub">累计分配 {{ metrics.memory.totalAllocMb }} MB</div>
          </div>
          <div class="m-card">
            <div class="m-label">堆对象 / GC</div>
            <div class="m-value">{{ metrics.memory.heapObjects }}</div>
            <div class="m-sub">GC {{ metrics.memory.gcCycles }} 次 · 系统 {{ metrics.memory.sysMb }} MB</div>
          </div>
          <div class="m-card">
            <div class="m-label">Goroutine</div>
            <div class="m-value">{{ metrics.runtime.goroutines }}</div>
            <div class="m-sub">并发协程数</div>
          </div>
          <div class="m-card">
            <div class="m-label">CPU 核心</div>
            <div class="m-value">{{ metrics.runtime.cpus }}</div>
            <div class="m-sub">可用逻辑核心</div>
          </div>
        </div>
      </div>

      <!-- 热门路径 -->
      <div class="metrics-section" v-if="metrics.topPaths.length">
        <div class="section-title">请求路径 Top {{ metrics.topPaths.length }}</div>
        <div class="m-card m-card-wide">
          <div class="path-row" v-for="p in metrics.topPaths" :key="p.path">
            <div class="path-name" :title="p.path">{{ p.path }}</div>
            <div class="path-bar-wrap">
              <div
                class="path-bar"
                :style="{ width: (p.count / maxPathCount * 100).toFixed(1) + '%' }"
              ></div>
            </div>
            <div class="path-count">{{ p.count }}</div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page {
  min-height: 100vh;
}
.metrics-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 24px 16px;
}
.metrics-title {
  font-size: 20px;
  font-weight: 600;
}
.metrics-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.metrics-loading {
  padding: 80px 24px;
  text-align: center;
  color: var(--dim);
}
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  padding: 0 24px;
  margin-bottom: 16px;
}
.metrics-grid.two {
  grid-template-columns: repeat(2, 1fr);
}
.m-card {
  background: var(--card);
  border-radius: 10px;
  padding: 16px;
  min-width: 0;
}
.m-card-wide {
  margin: 0 24px;
}
.m-label {
  font-size: 12px;
  color: var(--dim);
  margin-bottom: 6px;
}
.m-value {
  font-size: 26px;
  font-weight: 700;
  color: #fff;
}
.m-value.small {
  font-size: 22px;
}
.m-value .unit {
  font-size: 14px;
  font-weight: 400;
  color: var(--sub);
  margin-left: 2px;
}
.m-sub {
  font-size: 11px;
  color: var(--dim);
  margin-top: 4px;
}
.metrics-section {
  margin-bottom: 20px;
}
.section-title {
  font-size: 14px;
  color: var(--sub);
  padding: 0 24px 10px;
  font-weight: 600;
}
.latency-row {
  display: flex;
  gap: 32px;
}
.latency-item {
  flex: 1;
}
.path-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 0;
}
.path-name {
  width: 160px;
  font-size: 12px;
  color: var(--sub);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 0;
}
.path-bar-wrap {
  flex: 1;
  height: 8px;
  background: var(--white-10, rgba(255, 255, 255, 0.08));
  border-radius: 4px;
  overflow: hidden;
}
.path-bar {
  height: 100%;
  background: linear-gradient(90deg, #6c5ce7, #a29bfe);
  border-radius: 4px;
  transition: width 0.4s ease;
}
.path-count {
  width: 60px;
  text-align: right;
  font-size: 12px;
  color: var(--dim);
  flex-shrink: 0;
}

/* 移动端响应式 */
@media (max-width: 640px) {
  .metrics-grid {
    grid-template-columns: repeat(2, 1fr);
    padding: 0 16px;
  }
  .metrics-grid.two {
    grid-template-columns: repeat(2, 1fr);
  }
  .m-card-wide {
    margin: 0 16px;
  }
  .section-title {
    padding: 0 16px 10px;
  }
  .path-name {
    width: 100px;
  }
  .latency-row {
    gap: 16px;
  }
  .metrics-head {
    padding: 8px 16px 16px;
  }
}
</style>
