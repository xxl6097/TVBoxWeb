<template>
  <div class="up-wrap">

    <!-- 版本信息行 -->
    <div class="up-info">
      <div class="up-info-item">
        <span class="up-info-label">当前版本</span>
        <span class="up-info-val">{{ currentVersion }}</span>
      </div>
      <div class="up-info-item">
        <span class="up-info-label">构建时间</span>
        <span class="up-info-val">{{ buildTime }}</span>
      </div>
      <div class="up-info-item">
        <span class="up-info-label">平台架构</span>
        <span class="up-info-val">{{ osArch }}</span>
      </div>
    </div>

    <!-- 检查按钮 -->
    <div class="up-actions">
      <button
        class="up-btn up-btn-check"
        :class="{ spinning: checking }"
        :disabled="checking || upgrading"
        @click="checkUpdate"
      >
        <span class="up-btn-icon">↻</span>
        <span>{{ checking ? '检查中…' : '检查更新' }}</span>
      </button>
    </div>

    <!-- 错误 -->
    <div v-if="error" class="up-row up-row-error">
      <span class="up-row-icon">⚠</span>
      <span>{{ error }}</span>
    </div>

    <!-- 已是最新 -->
    <div v-else-if="checked && !updateAvailable && !upgrading && !upgraded" class="up-row up-row-ok">
      <span class="up-row-icon">✓</span>
      <span>已是最新版本</span>
    </div>

    <!-- 发现新版本 -->
    <div v-else-if="updateAvailable && !upgrading && !upgraded" class="up-notice">
      <div class="up-notice-head">
        <span class="up-notice-badge">NEW</span>
        <span class="up-notice-ver">发现新版本 <strong>{{ latestVersion }}</strong></span>
        <span v-if="publishedAt" class="up-notice-date">{{ fmtDate(publishedAt) }}</span>
        <span v-if="size" class="up-notice-size">{{ fmtSize(size) }}</span>
      </div>

      <!-- release notes -->
      <div v-if="releaseNotes" class="up-notes">
        <div class="up-notes-content" v-html="releaseNotes"></div>
      </div>

      <!-- 操作 -->
      <div class="up-upgrade-row">
        <!-- 状态：初始 → 待确认（按钮变红高亮）→ 升级中 -->
        <button
          class="up-btn"
          :class="confirming ? 'up-btn-confirm' : 'up-btn-primary'"
          @click="confirmUpgrade"
        >
          <span>{{ confirming ? '⚠ 再次点击确认升级' : '立即升级' }}</span>
        </button>
        <a v-if="pageUrl" :href="pageUrl" target="_blank" class="up-link">查看发布页 ›</a>
      </div>
      <p v-if="confirming" class="up-confirm-tip">升级将替换当前程序文件，完成后需手动重启服务。</p>
    </div>

    <!-- 升级进度 -->
    <div v-else-if="upgrading" class="up-progress">
      <div class="up-progress-head">
        <span class="up-progress-label">{{ stageText }}</span>
        <span v-if="loaded > 0 || total > 0" class="up-progress-size">{{ fmtSize(loaded) }}{{ total > 0 ? ' / ' + fmtSize(total) : '' }}</span>
      </div>
      <div class="up-bar">
        <div class="up-bar-fill" :style="{ width: progressPct + '%' }"></div>
      </div>
      <p v-if="progressMsg" class="up-progress-msg">{{ progressMsg }}</p>
    </div>

    <!-- 升级完成 -->
    <div v-else-if="upgraded" class="up-row up-row-done">
      <span class="up-row-icon">✓</span>
      <span>{{ doneMessage || '升级完成，请重启服务生效' }}</span>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const currentVersion = ref('…')
const buildTime = ref('…')
const osArch = ref('…')
const checking = ref(false)
const checked = ref(false)
const updateAvailable = ref(false)
const latestVersion = ref('')
const publishedAt = ref('')
const releaseNotes = ref('')
const pageUrl = ref('')
const size = ref(0)
const error = ref('')

const upgrading = ref(false)
const confirming = ref(false)
let confirmTimer = 0
const upgraded = ref(false)
const doneMessage = ref('')
const stageText = ref('')
const progressMsg = ref('')
const loaded = ref(0)
const total = ref(0)
const progressPct = ref(0)

onMounted(loadVersion)

async function loadVersion() {
  try {
    const res = await fetch('/api/version')
    const data = await res.json()
    currentVersion.value = data.appVersion || '未知'
    buildTime.value = data.buildTime ? data.buildTime.replace('T', ' ').replace('Z', '') : '未知'
    osArch.value = `${data.osType || '?'}/${data.arch || '?'}`
  } catch {
    currentVersion.value = '未知'
  }
}

async function checkUpdate() {
  checking.value = true
  checked.value = false
  updateAvailable.value = false
  error.value = ''
  upgraded.value = false

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
  } catch {
    error.value = '检查更新失败，请稍后重试'
  } finally {
    checking.value = false
  }
}

// 修复：confirming 时不能 disabled，两次点击才执行
function confirmUpgrade() {
  if (!confirming.value) {
    confirming.value = true
    clearTimeout(confirmTimer)
    confirmTimer = window.setTimeout(() => { confirming.value = false }, 6000)
    return
  }
  clearTimeout(confirmTimer)
  confirming.value = false
  runUpgrade()
}

async function runUpgrade() {
  upgrading.value = true
  updateAvailable.value = false
  error.value = ''
  stageText.value = '准备中…'
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
      for (const evt of events) handleSSE(evt)
    }
  } catch (err: any) {
    error.value = err.message || '升级失败'
    upgrading.value = false
  }
}

function handleSSE(raw: string) {
  let event = 'message'
  let data = ''
  for (const line of raw.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) data += line.slice(5).trim()
  }
  if (!data) return
  let payload: any
  try { payload = JSON.parse(data) } catch { return }

  switch (event) {
    case 'progress': applyProgress(payload); break
    case 'done':
      upgrading.value = false
      upgraded.value = true
      doneMessage.value = payload.message || '升级完成，请重启服务生效'
      break
    case 'error':
      upgrading.value = false
      error.value = payload.message || '升级失败'
      break
  }
}

function applyProgress(p: any) {
  const stageMap: Record<string, string> = {
    checking: '检查版本…',
    downloading: '下载中…',
    verifying: '校验 sha256…',
    replacing: '替换文件…',
    done: '升级完成',
  }
  stageText.value = stageMap[p.stage] || p.stage || '处理中…'
  if (p.stage === 'downloading') {
    loaded.value = p.loaded || 0
    total.value = p.total || 0
    if (p.total > 0) progressPct.value = Math.min(95, Math.round((p.loaded / p.total) * 100))
  } else if (p.stage === 'verifying') {
    progressPct.value = 97
  } else if (p.stage === 'replacing' || p.stage === 'done') {
    progressPct.value = 100
  }
  progressMsg.value = p.message || ''
}

function fmtSize(bytes: number): string {
  if (!bytes || bytes <= 0) return ''
  const u = ['B', 'KB', 'MB', 'GB']
  let i = 0, n = bytes
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return n.toFixed(1) + ' ' + u[i]
}

function fmtDate(iso: string): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })
  } catch { return iso }
}

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
/* ======== 容器 ======== */
.up-wrap {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ======== 版本信息卡 ======== */
.up-info {
  display: flex;
  flex-direction: column;
  gap: 0;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}
.up-info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  font-size: 13px;
}
.up-info-item + .up-info-item {
  border-top: 1px solid var(--border);
}
.up-info-label { color: var(--sub); }
.up-info-val   { color: var(--txt); font-weight: 500; font-variant-numeric: tabular-nums; }

/* ======== 按钮区 ======== */
.up-actions { display: flex; gap: 10px; }

.up-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 9px 20px;
  border-radius: var(--radius);
  font-size: 14px;
  font-weight: 500;
  border: 1px solid transparent;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s, opacity 0.15s;
  line-height: 1;
  white-space: nowrap;
}
.up-btn:disabled { opacity: 0.45; cursor: not-allowed; }

/* 次要（检查更新） */
.up-btn-check {
  background: var(--white-15);
  border-color: var(--border);
  color: var(--txt);
}
.up-btn-check:hover:not(:disabled) {
  background: var(--white-30);
  border-color: var(--sub);
}

/* 主要（立即升级） */
.up-btn-primary {
  background: #fff;
  color: #141218;
  border-color: #fff;
}
.up-btn-primary:hover:not(:disabled) {
  background: #e2e2e2;
  border-color: #e2e2e2;
}

/* 确认态（红色高亮，可点击） */
.up-btn-confirm {
  background: transparent;
  border-color: var(--warn);
  color: var(--warn);
  animation: confirm-pulse 1s ease-in-out infinite;
}
.up-btn-confirm:hover {
  background: rgba(255, 180, 171, 0.12);
}

@keyframes confirm-pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(255, 180, 171, 0); }
  50%       { box-shadow: 0 0 0 4px rgba(255, 180, 171, 0.25); }
}

/* 旋转图标 */
.up-btn-icon { font-size: 15px; display: inline-block; }
.spinning .up-btn-icon { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

/* ======== 状态行 ======== */
.up-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: var(--radius);
  font-size: 14px;
  border: 1px solid var(--border);
}
.up-row-icon { font-size: 16px; flex-shrink: 0; }
.up-row-ok   { color: #7ee787; border-color: rgba(126, 231, 135, 0.3); background: rgba(126, 231, 135, 0.06); }
.up-row-done { color: #7ee787; border-color: rgba(126, 231, 135, 0.3); background: rgba(126, 231, 135, 0.06); }
.up-row-error { color: var(--warn); border-color: rgba(255, 180, 171, 0.3); background: rgba(255, 180, 171, 0.06); }

/* ======== 新版本通知 ======== */
.up-notice {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}

.up-notice-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.up-notice-badge {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.06em;
  padding: 2px 7px;
  border-radius: 4px;
  background: var(--selected);
  color: #141218;
  flex-shrink: 0;
}
.up-notice-ver   { font-size: 15px; font-weight: 600; flex-shrink: 0; }
.up-notice-date  { font-size: 12px; color: var(--dim); margin-left: auto; }
.up-notice-size  { font-size: 12px; color: var(--dim); flex-shrink: 0; }

/* release notes */
.up-notes {
  padding: 14px 16px;
  border-bottom: 1px solid var(--border);
  max-height: 200px;
  overflow-y: auto;
}
.up-notes-content {
  font-size: 13px;
  line-height: 1.7;
  color: var(--sub);
}
.up-notes-content :deep(h2),
.up-notes-content :deep(h3),
.up-notes-content :deep(h4) { margin: 8px 0 4px; color: var(--txt); font-size: 13px; }
.up-notes-content :deep(li)  { margin-left: 16px; margin-bottom: 2px; list-style: disc; }
.up-notes-content :deep(strong) { color: var(--txt); }
.up-notes-content :deep(code) {
  background: var(--white-15);
  padding: 1px 5px;
  border-radius: 3px;
  font-family: monospace;
  font-size: 12px;
}

/* 操作区 */
.up-upgrade-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 16px;
}
.up-link {
  font-size: 13px;
  color: var(--dim);
  text-decoration: none;
  transition: color 0.12s;
}
.up-link:hover { color: var(--sub); }

/* 确认提示文字 */
.up-confirm-tip {
  margin: 0;
  padding: 0 16px 12px;
  font-size: 12px;
  color: var(--warn);
  opacity: 0.8;
}

/* ======== 进度条 ======== */
.up-progress {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.up-progress-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 14px;
}
.up-progress-label { font-weight: 500; }
.up-progress-size  { font-size: 12px; color: var(--dim); font-variant-numeric: tabular-nums; }

.up-bar {
  height: 4px;
  background: var(--white-15);
  border-radius: 2px;
  overflow: hidden;
}
.up-bar-fill {
  height: 100%;
  background: #fff;
  border-radius: 2px;
  transition: width 0.4s ease;
}

.up-progress-msg {
  margin: 0;
  font-size: 12px;
  color: var(--dim);
}

/* ======== 响应式 ======== */
@media (max-width: 640px) {
  .up-info-item { padding: 10px 14px; }
  .up-notice-date { display: none; }
}
</style>
