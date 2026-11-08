<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { api } from '@/api'
import { useAppStore } from '@/stores/app'
import type { FavItem, FavGroup } from '@/types'
import TopBar from '@/components/TopBar.vue'

const router = useRouter()
const app = useAppStore()
const favs = ref<FavItem[]>([])
const groups = ref<Array<FavGroup & { count: number }>>([])
const defaultCount = ref(0)
const selectedGroupId = ref<string | null>(null) // null=全部, ''=未分组, 'xxx'=指定分组
const moveMenuFav = ref<FavItem | null>(null)

onMounted(load)

async function load() {
  favs.value = await api.favorites()
  const r = await api.favGroups()
  groups.value = r.groups.map(g => ({ ...g, count: g.count ?? 0 }))
  defaultCount.value = r.defaultCount
}

const filteredFavs = computed<FavItem[]>(() => {
  if (selectedGroupId.value === null) return favs.value
  if (selectedGroupId.value === '') return favs.value.filter(f => !f.groupId)
  return favs.value.filter(f => f.groupId === selectedGroupId.value)
})

function selectGroup(id: string | null) {
  selectedGroupId.value = id
}

function open(f: FavItem) {
  router.push({ name: 'detail', query: { site: f.siteKey, siteName: f.siteName, id: f.vodId } })
}

async function remove(f: FavItem) {
  await api.toggleFavorite(f)
  app.showTip('已取消收藏')
  load()
}

async function createGroup() {
  try {
    const { value } = await ElMessageBox.prompt('请输入分组名称', '新建分组', {
      confirmButtonText: '创建', cancelButtonText: '取消',
      inputPattern: /\S+/, inputErrorMessage: '名称不能为空'
    })
    if (value) {
      await api.createFavGroup(value.trim())
      app.showTip('分组创建成功')
      load()
    }
  } catch { /* 用户取消 */ }
}

async function renameGroup(g: FavGroup & { count: number }) {
  try {
    const { value } = await ElMessageBox.prompt('请输入新名称', '重命名分组', {
      confirmButtonText: '确定', cancelButtonText: '取消',
      inputValue: g.name, inputPattern: /\S+/, inputErrorMessage: '名称不能为空'
    })
    if (value && value !== g.name) {
      await api.updateFavGroup(g.id, value.trim())
      app.showTip('重命名成功')
      load()
    }
  } catch { /* 用户取消 */ }
}

async function deleteGroup(g: FavGroup & { count: number }) {
  try {
    await ElMessageBox.confirm(
      `确定删除分组「${g.name}」？其中 ${g.count} 部收藏将移回未分组。`,
      '删除分组', { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
    await api.deleteFavGroup(g.id)
    app.showTip('分组已删除')
    if (selectedGroupId.value === g.id) selectedGroupId.value = null
    load()
  } catch { /* 用户取消 */ }
}

async function moveTo(f: FavItem, groupId: string) {
  moveMenuFav.value = null
  await api.moveFav(f.siteKey, f.vodId, groupId)
  app.showTip('已移动')
  load()
}

function toggleMoveMenu(f: FavItem) {
  moveMenuFav.value = moveMenuFav.value?.vodId === f.vodId ? null : f
}

function currentGroupName(): string {
  if (selectedGroupId.value === null) return '全部收藏'
  if (selectedGroupId.value === '') return '未分组'
  return groups.value.find(g => g.id === selectedGroupId.value)?.name || '未知分组'
}
</script>

<template>
  <div class="page" @click="moveMenuFav = null">
    <TopBar />
    <div class="collect-layout">
      <!-- 左侧分组导航 -->
      <div class="sidebar">
        <div class="sidebar-title">收藏分组</div>
        <div class="group-item" :class="{ active: selectedGroupId === null }" @click="selectGroup(null)">
          <span class="group-name">全部</span>
          <span class="group-count">{{ favs.length }}</span>
        </div>
        <div v-if="defaultCount > 0" class="group-item" :class="{ active: selectedGroupId === '' }" @click="selectGroup('')">
          <span class="group-name">未分组</span>
          <span class="group-count">{{ defaultCount }}</span>
        </div>
        <div
          v-for="g in groups" :key="g.id"
          class="group-item" :class="{ active: selectedGroupId === g.id }"
          @click="selectGroup(g.id)"
        >
          <span class="group-name">{{ g.name }}</span>
          <span class="group-count">{{ g.count }}</span>
          <div class="group-actions">
            <span class="group-action" title="重命名" @click.stop="renameGroup(g)">✎</span>
            <span class="group-action danger" title="删除" @click.stop="deleteGroup(g)">✕</span>
          </div>
        </div>
        <div class="group-add" @click="createGroup">+ 新建分组</div>
      </div>

      <!-- 右侧收藏内容 -->
      <div class="collect-main">
        <div class="collect-header">
          <span class="collect-title">{{ currentGroupName() }}</span>
          <span class="count">{{ filteredFavs.length }} 部</span>
        </div>
        <div v-if="filteredFavs.length" class="grid">
          <div v-for="f in filteredFavs" :key="f.siteKey + f.vodId" class="item">
            <div class="poster" @click="open(f)">
              <img :src="f.pic" :alt="f.name" loading="lazy" />
              <!-- 悬停操作菜单 -->
              <div class="item-overlay" @click.stop>
                <button class="act-btn del-btn" title="取消收藏" @click="remove(f)">✕</button>
                <button v-if="groups.length" class="act-btn move-btn" title="移到分组" @click.stop="toggleMoveMenu(f)">⬛</button>
              </div>
              <!-- 移动到分组菜单 -->
              <div v-if="moveMenuFav?.vodId === f.vodId && moveMenuFav?.siteKey === f.siteKey" class="move-menu" @click.stop>
                <div class="move-menu-title">移动到</div>
                <div class="move-menu-item" :class="{ active: !f.groupId }" @click="moveTo(f, '')">未分组</div>
                <div
                  v-for="g in groups" :key="g.id"
                  class="move-menu-item" :class="{ active: f.groupId === g.id }"
                  @click="moveTo(f, g.id)"
                >{{ g.name }}</div>
              </div>
            </div>
            <div class="name" @click="open(f)">{{ f.name }}</div>
            <div v-if="f.groupId" class="tag-group">
              {{ groups.find(g => g.id === f.groupId)?.name }}
            </div>
          </div>
        </div>
        <div v-else class="empty">
          {{ selectedGroupId === null ? '还没有收藏内容' : '该分组暂无收藏' }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.collect-layout {
  display: flex;
  min-height: calc(100vh - 56px);
}

/* 左侧 */
.sidebar {
  width: 200px;
  flex-shrink: 0;
  border-right: 1px solid var(--border);
  padding: 16px 0;
  overflow-y: auto;
}

.sidebar-title {
  padding: 0 16px 10px;
  font-size: 11px;
  color: var(--dim);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.group-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 16px;
  cursor: pointer;
  transition: background 0.12s;
}

.group-item:hover { background: var(--card); }

.group-item.active {
  background: var(--card);
  color: var(--primary, #409eff);
  font-weight: 600;
}

.group-item.active::before {
  content: '';
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 3px;
  background: var(--primary, #409eff);
  border-radius: 0 2px 2px 0;
}

.group-name { flex: 1; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.group-count { font-size: 11px; color: var(--dim); background: var(--card-hi); padding: 1px 6px; border-radius: 10px; flex-shrink: 0; }

.group-actions { display: none; gap: 4px; flex-shrink: 0; }
.group-item:hover .group-actions { display: flex; }

.group-action {
  width: 20px; height: 20px;
  display: flex; align-items: center; justify-content: center;
  border-radius: 4px; font-size: 11px; color: var(--dim); cursor: pointer;
}
.group-action:hover { background: var(--card-hi); color: var(--text); }
.group-action.danger:hover { background: rgba(255,77,79,.15); color: #ff4d4f; }

.group-add {
  padding: 10px 16px;
  cursor: pointer;
  font-size: 13px;
  color: var(--dim);
  border-top: 1px solid var(--border);
  margin-top: 8px;
}
.group-add:hover { color: var(--primary, #409eff); }

/* 右侧 */
.collect-main { flex: 1; min-width: 0; padding: 16px 0 0; }

.collect-header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 0 24px 16px;
}

.collect-title { font-size: 16px; font-weight: 600; }
.count { font-size: 13px; color: var(--dim); }

/* 收藏网格 */
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 16px;
  padding: 0 24px;
}

.item { cursor: pointer; }

.poster {
  position: relative;
  aspect-ratio: 2 / 3;
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--card);
  border: 2px solid transparent;
  transition: border-color 0.15s;
}
.item:hover .poster { border-color: rgba(255,255,255,0.5); }
.poster img { width: 100%; height: 100%; object-fit: cover; }

.item-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0,0,0,0);
  display: flex;
  align-items: flex-start;
  justify-content: flex-end;
  gap: 4px;
  padding: 6px;
  opacity: 0;
  transition: opacity 0.15s;
}
.poster:hover .item-overlay { opacity: 1; background: rgba(0,0,0,0.15); }

.act-btn {
  width: 26px; height: 26px;
  border: none;
  border-radius: 50%;
  background: rgba(0,0,0,0.65);
  color: #fff;
  font-size: 11px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  backdrop-filter: blur(4px);
}
.del-btn:hover { background: rgba(255,77,79,0.85); }
.move-btn:hover { background: rgba(64,158,255,0.85); }

/* 移动菜单 */
.move-menu {
  position: absolute;
  right: 6px; top: 36px;
  background: var(--bg, #1a1a2e);
  border: 1px solid var(--border);
  border-radius: 8px;
  min-width: 140px;
  z-index: 100;
  box-shadow: 0 4px 20px rgba(0,0,0,0.4);
  overflow: hidden;
}
.move-menu-title {
  padding: 8px 12px 6px;
  font-size: 11px;
  color: var(--dim);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.move-menu-item {
  padding: 8px 12px;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.1s;
}
.move-menu-item:hover { background: var(--card); }
.move-menu-item.active { color: var(--primary, #409eff); font-weight: 600; }

.name {
  margin-top: 8px;
  font-size: 14px;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}

.tag-group {
  display: inline-block;
  margin-top: 3px;
  font-size: 11px;
  color: var(--primary, #409eff);
  background: rgba(64,158,255,0.1);
  padding: 1px 7px;
  border-radius: 8px;
  max-width: 100%;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}

.empty { text-align: center; padding: 60px 24px; color: var(--dim); }

/* ===== 移动端 ===== */
@media (max-width: 768px) {
  .collect-layout { flex-direction: column; }

  .sidebar {
    width: 100%;
    border-right: none;
    border-bottom: 1px solid var(--border);
    padding: 8px 0;
    display: flex;
    overflow-x: auto;
    white-space: nowrap;
  }
  .sidebar-title { display: none; }

  .group-item {
    flex-shrink: 0;
    padding: 5px 12px;
    border-radius: 20px;
    margin: 4px 2px;
    border: 1px solid var(--border);
  }
  .group-item.active {
    border-color: var(--primary, #409eff);
    background: rgba(64,158,255,0.1);
  }
  .group-item.active::before { display: none; }
  .group-actions { display: none !important; }

  .group-add {
    flex-shrink: 0;
    border-top: none;
    margin: 4px 2px;
    padding: 5px 12px;
    border-radius: 20px;
    border: 1px dashed var(--border);
  }

  .grid {
    grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
    gap: 10px;
    padding: 0 12px;
  }
  .item-overlay { opacity: 1; }
}
</style>
