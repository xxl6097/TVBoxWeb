<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import type { Vod } from '@/types'
import TopBar from '@/components/TopBar.vue'
import VodGrid from '@/components/VodGrid.vue'
import SkeletonList from '@/components/SkeletonList.vue'

const router = useRouter()
const app = useAppStore()

const kw = ref('')
const results = ref<Vod[]>([])
const searching = ref(false)
const searched = ref(false)
const keywords = ref<string[]>([])
const siteInfo = ref('')
const sugg = ref<string[]>([])
const suggLoading = ref(false)

onMounted(async () => {
  keywords.value = await api.keywords()
})

// 按站源分组结果
const grouped = computed(() => {
  const map = new Map<string, Vod[]>()
  for (const v of results.value) {
    const key = v.siteName || '其他'
    if (!map.has(key)) map.set(key, [])
    map.get(key)!.push(v)
  }
  return [...map.entries()]
})

async function doSearch(k?: string) {
  const word = (k ?? kw.value).trim()
  if (!word) {
    app.showTip('请输入关键词')
    return
  }
  kw.value = word
  sugg.value = []
  searching.value = true
  searched.value = true
  try {
    const r = await api.search(word)
    results.value = r.list
    siteInfo.value = `${r.sites}/${r.total} 个站源有响应`
    keywords.value = await api.pushKeyword(word)
  } catch (e: any) {
    app.showTip('搜索失败：' + e.message)
  } finally {
    searching.value = false
  }
}

function clearKw() {
  kw.value = ''
  results.value = []
  searched.value = false
  sugg.value = []
}

function clearKeywords() {
  api.clearKeywords().then(() => {
    keywords.value = []
    app.showTip('已清空搜索记录')
  })
}

// 实时联想
let suggTimer = 0
let suggAbort: AbortController | null = null

function onInput() {
  const w = kw.value.trim()
  if (!w) {
    sugg.value = []
    return
  }
  if (suggTimer) clearTimeout(suggTimer)
  // abort 之前还未完成的联想请求，防止旧请求结果覆盖新输入的结果
  if (suggAbort) suggAbort.abort()
  suggTimer = window.setTimeout(async () => {
    const ctrl = new AbortController()
    suggAbort = ctrl
    try {
      sugg.value = await api.suggest(w)
    } catch (e: any) {
      if (e?.name !== 'AbortError') sugg.value = []
    } finally {
      if (suggAbort === ctrl) suggAbort = null
    }
  }, 350) // 350ms 防抖，减少打字过程中的无效请求
}
</script>

<template>
  <div class="page">
    <TopBar />
    <div class="search-box">
      <el-input
        v-model="kw"
        size="large"
        placeholder="输入片名，回车搜索（支持拼音首字母，如 sgyy → 三国演义）"
        clearable
        @keyup.enter="doSearch()"
        @input="onInput"
        @clear="clearKw"
      >
        <template #append>
          <el-button :loading="searching" @click="doSearch()">搜索</el-button>
        </template>
      </el-input>
    </div>

    <!-- 猜你想搜 -->
    <div v-if="sugg.length && !searched" class="sugg-box">
      <div class="kw-head">猜你想搜</div>
      <div class="sugg-chips">
        <span v-for="s in sugg" :key="s" class="chip" @click="doSearch(s)">{{ s }}</span>
      </div>
    </div>

    <div v-if="!searched && !sugg.length && keywords.length" class="kw-box">
      <div class="kw-head">
        <span>最近搜过</span>
        <span class="clear" @click="clearKeywords">清空记录</span>
      </div>
      <div class="kw-chips">
        <span v-for="k in keywords" :key="k" class="chip" @click="doSearch(k)">{{ k }}</span>
      </div>
    </div>

    <div v-if="!searched && !sugg.length && !keywords.length" class="empty-guide">
      <div class="guide-title">搜索影片</div>
      <p class="guide-sub">会在已启用的协议型站源里并发检索（最多 18 个）</p>
      <p class="guide-sub">支持拼音首字母，如输入 sgyy 搜「三国演义」</p>
    </div>

    <div v-if="searched">
      <div class="section-title">
        搜索结果
        <span v-if="siteInfo" class="count">{{ siteInfo }}</span>
        <span v-if="results.length" class="count">共 {{ results.length }} 条</span>
      </div>
      <!-- 加载中：骨架屏 -->
      <SkeletonList v-if="searching" type="search" :count="10" />
      <!-- 搜索结果 -->
      <div v-else-if="results.length">
        <div v-for="[site, vods] in grouped" :key="site" class="group">
          <div class="group-head">{{ site }} <span class="count">{{ vods.length }} 个结果</span></div>
          <VodGrid :vods="vods" />
        </div>
      </div>
      <div v-else class="empty">没有搜到结果（站源可能已失效，或需要爬虫运行时）</div>
    </div>
  </div>
</template>

<style scoped>
.search-box {
  padding: 16px 24px;
  max-width: 640px;
}
.sugg-box,
.kw-box {
  padding: 0 24px 16px;
}
.kw-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
  font-size: 14px;
  color: var(--sub);
}
.clear {
  color: var(--dim);
  cursor: pointer;
}
.sugg-chips,
.kw-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.group {
  margin-bottom: 20px;
}
.group-head {
  font-size: 15px;
  font-weight: 600;
  color: var(--sub);
  padding: 0 24px;
  margin-bottom: 10px;
}
.group-head .count {
  font-size: 12px;
  color: var(--dim);
  font-weight: 400;
}
.empty {
  text-align: center;
  padding: 60px 24px;
  color: var(--dim);
}
.empty-guide {
  text-align: center;
  padding: 60px 24px;
}
.guide-title {
  font-size: 22px;
  font-weight: 600;
  margin-bottom: 10px;
}
.guide-sub {
  font-size: 13px;
  color: var(--dim);
  margin: 4px 0;
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  .top { padding: 8px 16px; gap: 8px; }
  .hints { padding: 0 16px; }
  .hint-item { font-size: 13px; padding: 8px 10px; }
  .groups { padding: 0 16px; }
  .group-head { font-size: 16px; padding: 12px 0 6px; }
  .result-list { padding: 0 16px; }
  .result-row { padding: 8px 0; gap: 10px; }
  .result-thumb { width: 80px; flex-shrink: 0; }
  .result-info .name { font-size: 14px; }
  .result-info .meta { font-size: 12px; }
  .empty-guide { padding: 40px 16px; }
  .guide-title { font-size: 18px; }
}
</style>
