<template>
  <div class="upgrade-panel">
    <!-- 版本信息 -->
    <div class="version-info">
      <div class="info-row">
        <span class="label">当前版本</span>
        <span class="value">{{ currentVersion }}</span>
      </div>
      <div class="info-row">
        <span class="label">构建时间</span>
        <span class="value">{{ buildTime }}</span>
      </div>
      <div class="info-row">
        <span class="label">平台架构</span>
        <span class="value">{{ osArch }}</span>
      </div>
    </div>

    <!-- 检查更新按钮 -->
    <button
      class="check-btn"
      :class="{ loading: checking }"
      :disabled="checking || upgrading"
      @click="checkUpdate"
    >
      <span v-if="!checking">🔄 检查更新</span>
      <span v-else>检查中...</span>
    </button>

    <!-- 发现新版本 -->
    <div v-if="updateAvailable && !upgrading" class="update-notice">
      <div class="notice-header">
        <span class="icon">🎉</span>
        <span class="title">发现新版本 {{ latestVersion }}</span>
      </div>
      <div class="update-details">
        <p v-if="publishedAt"><strong>发布时间：</strong>{{ publishedAt }}</p>
        <p v-if="size"><strong>大小：</strong>{{ fmtSize(size) }}</p>
        <div v-if="releaseNotes" class="release-notes">
          <div class="notes-title">更新说明</div>
          <div class="notes-content" v-html="releaseNotes"></div>
        </div>
      </div>
      <div class="upgrade-actions">
        <button class="upgrade-btn" :disabled="confirming" @click="confirmUpgrade">
          {{ confirming ? '再次点击确认升级' : '立即升级' }}
        </button>
        <a v-if="pageUrl" :href="pageUrl" target="_blank" class="view-btn">查看发布页</a>
      </div>
    </div>

    <!-- 升级中 -->
    <div v-else-if="upgrading" class="upgrade-progress">
      <div class="progress-header">
        <span class="icon">⏳</span>
        <span class="title">{{ stageText }}</span>
      </div>
      <div class="progress-bar">
        <div class="progress-fill" :style="{ width: progressPct + '%' }"></div>
      </div>
      <div class="progress-meta">
        <span v-if="progressMsg">{{ progressMsg }}</span>
        <span v-if="loaded > 0 || total > 0">{{ fmtSize(loaded) }} / {{ total > 0 ? fmtSize(total) : '未知' }}</span>
      </div>
    </div>

    <!-- 升级完成 -->
    <div v-else-if="upgraded" class="up-to-date done">
      <span class="icon">✅</span>
      <span>{{ doneMessage }}</span>
    </div>

    <!-- 已是最新 -->
    <div v-else-if="checked && !updateAvailable" class="up-to-date">
      <span class="icon">✅</span>
      <span>已是最新版本</span>
    </div>

    <!-- 错误 -->
    <div v-if="error" class="error-notice">
      <span class="icon">⚠️</span>
      <span>{{ error }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const currentVersion = ref('')
const buildTime = ref('')
const osArch = ref('')
const checking = ref(false)
const checked = ref(false)
const updateAvailable = ref(false)
const latestVersion = ref('')
const publishedAt = ref('')
const releaseNotes = ref('')
const pageUrl = ref('')
const size = ref(0)
const error = ref('')

// 升级状态
const upgrading = ref(false)
const confirming = ref(false)
const upgraded = ref(false)
const doneMessage = ref('')
const stageText = ref('')
const progressMsg = ref('')
const loaded = ref(0)
const total = ref(0)
const progressPct = ref(0)

onMounted(loadVersion)

// 获取版本信息
async function loadVersion() {
  try {
    const res = await fetch('/api/version')
    const data = await res.json()
    currentVersion.value = data.appVersion || 'Unknown'
    buildTime.value = data.buildTime || 'Unknown'
    osArch.value = `${data.osType}/${data.arch}`
  } catch {
    currentVersion.value = 'Unknown'
    buildTime.value = 'Unknown'
    osArch.value = 'Unknown'
  }
}

// 检查更新
async function checkUpdate() {
  checking.value = true
  checked.value = false
  updateAvailable.value = false
  error.value = ''

  try {
    const res = await fetch('/api/upgrade/check')
    const data = await res.json()
    checked.value = true

    if (data.error && !data.updateable) {
      error.value = data.error
    } else if (data.updateable && data.release) {
      updateAvailable.value = true
      latestVersion.value = data.release.version || data.latest || ''
      publishedAt.value = data.release.publishedAt || ''
      releaseNotes.value = formatMarkdown(data.release.notes || '')
      pageUrl.value = data.release.pageUrl || ''
      size.value = data.release.size || 0
    }
  } catch (err) {
    error.value = '检查更新失败，请稍后重试'
    console.error('检查更新失败:', err)
  } finally {
    checking.value = false
  }
}

// 确认升级（两次点击确认）
function confirmUpgrade() {
  if (!confirming.value) {
    confirming.value = true
    setTimeout(() => (confirming.value = false), 5000)
    return
  }
  confirming.value = false
  runUpgrade()
}

// 执行升级（SSE 进度）
async function runUpgrade() {
  upgrading.value = true
  updateAvailable.value = false
  error.value = ''
  stageText.value = '准备升级...'
  progressPct.value = 0
  loaded.value = 0
  total.value = 0

  try {
    const res = await fetch('/api/upgrade/run', { method: 'POST' })
    if (!res.ok) {
      const j = await res.json().catch(() => ({ error: '升级请求失败 (' + res.status + ')' }))
      throw new Error(j.error || '升级请求失败')
    }

    const reader = res.body!.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const events = buffer.split('\n\n')
      buffer = events.pop() || ''
      for (const evt of events) {
        handleSSE(evt)
      }
    }
  } catch (err: any) {
    error.value = err.message || '升级失败'
    upgrading.value = false
  }
}

// 解析单个 SSE 事件
function handleSSE(raw: string) {
  let event = 'message'
  let data = ''
  for (const line of raw.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) data += line.slice(5).trim()
  }
  if (!data) return
  let payload: any
  try {
    payload = JSON.parse(data)
  } catch {
    return
  }

  switch (event) {
    case 'progress':
      applyProgress(payload)
      break
    case 'done':
      upgrading.value = false
      upgraded.value = true
      doneMessage.value = payload.message || '升级完成'
      break
    case 'error':
      upgrading.value = false
      error.value = payload.message || '升级失败'
      break
  }
}

// 应用进度
function applyProgress(p: any) {
  const stage = p.stage || ''
  switch (stage) {
    case 'checking':
      stageText.value = '正在检查...'
      break
    case 'downloading':
      stageText.value = '正在下载...'
      loaded.value = p.loaded || 0
      total.value = p.total || 0
      if (p.total > 0) progressPct.value = Math.min(100, Math.round((p.loaded / p.total) * 100))
      break
    case 'verifying':
      stageText.value = '正在校验 sha256...'
      progressPct.value = 100
      break
    case 'replacing':
      stageText.value = '正在替换文件...'
      break
    case 'done':
      stageText.value = '升级完成'
      progressPct.value = 100
      break
  }
  progressMsg.value = p.message || ''
}

// 格式化体积
function fmtSize(bytes: number): string {
  if (!bytes || bytes <= 0) return ''
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let n = bytes
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return n.toFixed(1) + ' ' + units[i]
}

// 简单的 Markdown 转换（转义 HTML，只认标题/列表/加粗/链接）
function formatMarkdown(text: string): string {
  if (!text) return ''
  const esc = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return esc
    .replace(/^### (.+)$/gm, '<h4>$1</h4>')
    .replace(/^## (.+)$/gm, '<h3>$1</h3>')
    .replace(/^# (.+)$/gm, '<h2>$1</h2>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/^[-*] (.+)$/gm, '<li>$1</li>')
    .replace(/\n/g, '<br>')
}
</script>

<style scoped>
.upgrade-panel {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 600px;
  margin: 0 auto;
}

.version-info {
  background: var(--card, #1a1a1a);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 14px;
}

.label { color: var(--sub, #999); }
.value { color: var(--text, #fff); font-weight: 500; }

.check-btn {
  padding: 12px 24px;
  background: var(--primary, #409eff);
  color: white;
  border: none;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}
.check-btn:hover:not(:disabled) { background: var(--primary-hover, #337ecc); transform: translateY(-1px); }
.check-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.check-btn.loading { animation: pulse 1.5s ease-in-out infinite; }

@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }

.update-notice {
  background: var(--card, #1a1a1a);
  border: 1px solid var(--primary, #409eff);
  border-radius: 12px;
  padding: 20px;
  color: var(--text, #fff);
}

.notice-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
  font-size: 18px;
  font-weight: 600;
}
.icon { font-size: 22px; }

.update-details { margin-bottom: 16px; font-size: 14px; line-height: 1.6; }
.update-details p { margin: 8px 0; }

.release-notes {
  margin-top: 12px;
  padding: 12px;
  background: var(--white-10, rgba(255,255,255,0.05));
  border-radius: 8px;
  max-height: 240px;
  overflow-y: auto;
}
.notes-title { font-size: 13px; color: var(--sub, #999); margin-bottom: 8px; }
.notes-content { font-size: 13px; line-height: 1.6; }
.notes-content :deep(h2), .notes-content :deep(h3), .notes-content :deep(h4) { margin: 8px 0 4px; }
.notes-content :deep(li) { margin-left: 16px; }
.notes-content :deep(code) { background: rgba(255,255,255,0.1); padding: 1px 5px; border-radius: 3px; }

.upgrade-actions { display: flex; gap: 10px; align-items: center; }
.upgrade-btn {
  padding: 10px 20px;
  background: var(--primary, #409eff);
  color: #fff;
  border: none;
  border-radius: 6px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}
.upgrade-btn:hover { transform: translateY(-1px); }
.upgrade-btn:disabled { opacity: 0.7; }
.view-btn { color: var(--sub, #999); font-size: 13px; }

.upgrade-progress {
  background: var(--card, #1a1a1a);
  border-radius: 12px;
  padding: 20px;
}
.progress-header { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; font-size: 15px; }
.progress-bar { height: 8px; background: var(--white-10, rgba(255,255,255,0.1)); border-radius: 4px; overflow: hidden; }
.progress-fill { height: 100%; background: var(--primary, #409eff); border-radius: 4px; transition: width 0.3s; }
.progress-meta { display: flex; justify-content: space-between; margin-top: 8px; font-size: 12px; color: var(--sub, #999); }

.up-to-date {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px;
  background: var(--card, #1a1a1a);
  border-radius: 8px;
  color: var(--text, #fff);
}
.up-to-date.done { border: 1px solid #52c41a; }

.error-notice {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px;
  background: #ff4d4f;
  color: white;
  border-radius: 8px;
}

@media (max-width: 640px) {
  .upgrade-panel { padding: 0; }
  .info-row { font-size: 13px; }
  .update-notice { padding: 16px; }
}
</style>
