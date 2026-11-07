<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import type { Site, Category, VodPage, FilterGroup } from '@/types'
import TopBar from '@/components/TopBar.vue'
import VodGrid from '@/components/VodGrid.vue'

const route = useRoute()
const router = useRouter()
const app = useAppStore()

const sites = ref<Site[]>([])
const siteKey = ref((route.query.site as string) || '')
const categories = ref<Category[]>([])
const page = ref<VodPage>({ items: [], page: 1, pageCount: 1, total: 0 })
const curPage = ref(1)
const selectedParent = ref('')
const selectedChild = ref('')
const filters = ref<FilterGroup[]>([])
const filterVals = ref<Record<string, string>>({})
const loading = ref(false)

const currentSite = computed(() => sites.value.find((s) => s.key === siteKey.value))

// 顶层分类（pid 为空且 id 不为空，过滤掉后端注入的虚拟「全部」条目）
const topCats = computed(() => categories.value.filter((c) => c.pid === '' && c.id !== ''))
const childrenOf = (tid: string) => categories.value.filter((c) => c.pid === tid)

onMounted(async () => {
  const list = await api.sites()
  sites.value = list.filter((s) => s.usable)
  if (!siteKey.value && sites.value.length) siteKey.value = sites.value[0].key
  if (siteKey.value) await loadCategories()
})

watch(siteKey, () => {
  if (route.query.site !== siteKey.value) {
    router.replace({ query: { ...route.query, site: siteKey.value } })
  }
  selectedParent.value = ''
  selectedChild.value = ''
  filterVals.value = {}
  loadCategories()
})

async function loadCategories() {
  loading.value = true
  try {
    categories.value = await api.categories(siteKey.value)
  } catch (e: any) {
    app.showTip('分类加载失败：' + e.message)
  } finally {
    loading.value = false
  }
  // 分类就绪后，加载默认「全部」分类的影片列表
  load()
}

function selectParent(tid: string) {
  selectedParent.value = tid
  selectedChild.value = ''
  filterVals.value = {}
  curPage.value = 1
  load()
  // 更新筛选
  if (currentSite.value) {
    filters.value = parseFilters(currentSite.value.ext, tid)
  }
}

function selectChild(tid: string) {
  selectedChild.value = tid
  curPage.value = 1
  load()
}

function currentTid(): string {
  if (selectedChild.value) return selectedChild.value
  return selectedParent.value
}

function currentChildren(): string {
  if (selectedChild.value) return ''
  // 「全部」分类直接取全集，不聚合子分类
  if (!selectedParent.value) return ''
  return childrenOf(selectedParent.value).map((c) => c.id).join(',')
}

async function load() {
  if (!siteKey.value) return
  loading.value = true
  try {
    const ext = buildExt()
    page.value = await api.vodList(siteKey.value, currentTid(), curPage.value, ext, currentChildren())
  } catch (e: any) {
    app.showTip('列表加载失败：' + e.message)
  } finally {
    loading.value = false
  }
}

function buildExt(): string {
  const parts = Object.entries(filterVals.value).filter(([, v]) => v !== '')
  if (!parts.length) return ''
  return '&' + parts.map(([k, v]) => `${k}=${encodeURIComponent(v)}`).join('&')
}

function applyFilter(key: string, v: string) {
  filterVals.value[key] = v
  curPage.value = 1
  load()
}

function setFilterVal(key: string, v: string) {
  applyFilter(key, v)
}

function goto(p: number) {
  if (p < 1 || p > page.value.pageCount) return
  curPage.value = p
  load()
}

// 解析站点 ext 里的 filters（与后端 cms.FiltersOf 同逻辑）
function parseFilters(ext: string, tid: string): FilterGroup[] {
  const out: FilterGroup[] = []
  if (!ext || ext.trim()[0] !== '{') return out
  try {
    const j = JSON.parse(ext)
    const map = j.filters
    if (!map) return out
    const groups = map[tid]
    if (!groups) return out
    for (const g of groups) {
      const gg: FilterGroup = { key: g.key, name: g.name || g.key, values: [] }
      for (const v of g.value || []) {
        if (v.n) gg.values.push({ n: v.n, v: v.v })
      }
      if (gg.key && gg.values.length) out.push(gg)
    }
  } catch {
    /* ignore */
  }
  return out
}
</script>

<template>
  <div class="page">
    <TopBar />

    <div class="vod-head">
      <div class="site-row">
        <span class="site-name">{{ currentSite?.name || '点播' }}</span>
        <span v-if="page.total > 0" class="total-hint">共 {{ page.total.toLocaleString() }} 部</span>
        <el-select v-model="siteKey" size="small" placeholder="换站" style="width: 200px">
          <el-option v-for="s in sites" :key="s.key" :label="s.name" :value="s.key" />
        </el-select>
        <el-button size="small" @click="load">刷新</el-button>
      </div>

      <!-- 父分类 -->
      <div class="cats">
        <span class="chip" :class="{ active: selectedParent === '' && !selectedChild }" @click="selectParent('')">全部</span>
        <span
          v-for="c in topCats"
          :key="c.id"
          class="chip"
          :class="{ active: selectedParent === c.id && !selectedChild }"
          @click="selectParent(c.id)"
        >
          {{ c.name }}
        </span>
      </div>

      <!-- 子分类（只在选中具体父分类时展开，「全部」='' 不触发子分类展开） -->
      <div v-if="selectedParent !== '' && childrenOf(selectedParent).length" class="cats sub">
        <!-- 子分类「全部」：显示父分类下全部内容，清空子分类选中 -->
        <span
          class="chip"
          :class="{ active: !selectedChild }"
          @click="selectedChild = ''; load()"
        >全部</span>
        <span
          v-for="c in childrenOf(selectedParent)"
          :key="c.id"
          class="chip"
          :class="{ active: selectedChild === c.id }"
          @click="selectChild(c.id)"
        >
          {{ c.name }}
        </span>
      </div>

      <!-- 筛选 -->
      <div v-for="g in filters" :key="g.key" class="cats filter">
        <span class="filter-name">{{ g.name }}</span>
        <span
          v-for="v in g.values"
          :key="v.v"
          class="chip"
          :class="{ active: filterVals[g.key] === v.v }"
          @click="setFilterVal(g.key, v.v)"
        >
          {{ v.n }}
        </span>
      </div>
    </div>

    <div v-loading="loading">
      <VodGrid :vods="page.items" />
      <div v-if="!loading && !page.items.length" class="vod-empty">
        <p>暂无内容</p>
        <p class="hint">可能是这个站源当前分类下没有数据，换个分类或站源试试</p>
      </div>

      <div v-if="page.pageCount > 1" class="pager">
        <el-button size="small" :disabled="page.page <= 1" @click="goto(page.page - 1)">上一页</el-button>
        <span class="pager-info">{{ page.page }} / {{ page.pageCount }}</span>
        <el-button size="small" :disabled="page.page >= page.pageCount" @click="goto(page.page + 1)">下一页</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.vod-head {
  padding: 8px 24px 16px;
}
.site-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}
.site-name {
  font-size: 20px;
  font-weight: 600;
}
.total-hint {
  font-size: 13px;
  color: var(--dim);
  margin-right: 4px;
}
.vod-empty {
  text-align: center;
  padding: 60px 24px;
  color: var(--dim);
}
.vod-empty .hint {
  font-size: 13px;
  margin-top: 6px;
}
.cats {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}
.cats.sub {
  padding-left: 24px;
}
.cats.filter {
  align-items: center;
}
.filter-name {
  font-size: 13px;
  color: var(--dim);
  margin-right: 4px;
}
.pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 24px;
}
.pager-info {
  color: var(--sub);
  font-size: 14px;
}

/* ===== 移动端响应式 ===== */
@media (max-width: 640px) {
  .site-row { padding: 8px 16px; gap: 8px; flex-wrap: wrap; }
  .site-name { font-size: 18px; }
  .cats, .cats.sub {
    padding: 8px 16px;
    overflow-x: auto;
    flex-wrap: nowrap;
    padding-bottom: 8px;
  }
  .cats .chip, .cats.sub .chip { flex-shrink: 0; }
  .pager { padding: 16px; gap: 12px; }
  .vod-total { font-size: 13px; }
}
</style>
