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
      :disabled="checking"
      @click="checkUpdate"
    >
      <span v-if="!checking">🔄 检查更新</span>
      <span v-else>检查中...</span>
    </button>

    <!-- 更新提示 -->
    <div v-if="updateAvailable" class="update-notice">
      <div class="notice-header">
        <span class="icon">🎉</span>
        <span class="title">发现新版本</span>
      </div>
      <div class="update-details">
        <p><strong>最新版本：</strong>{{ latestVersion }}</p>
        <p><strong>发布时间：</strong>{{ publishedAt }}</p>
        <div v-if="releaseNotes" class="release-notes">
          <strong>更新说明：</strong>
          <div class="notes-content" v-html="releaseNotes"></div>
        </div>
      </div>
      <a :href="downloadUrl" target="_blank" class="download-btn">
        查看下载
      </a>
    </div>

    <!-- 已是最新版提示 -->
    <div v-else-if="checked && !updateAvailable" class="up-to-date">
      <span class="icon">✅</span>
      <span>已是最新版本</span>
    </div>

    <!-- 错误提示 -->
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
const downloadUrl = ref('')
const error = ref('')

// 获取版本信息
async function loadVersion() {
  try {
    const res = await fetch('/api/version')
    const data = await res.json()
    currentVersion.value = data.appVersion || 'Unknown'
    buildTime.value = data.buildTime || 'Unknown'
    osArch.value = `${data.osType}/${data.arch}`
  } catch (err) {
    console.error('获取版本信息失败:', err)
    currentVersion.value = 'v0.0.5'
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

    if (data.error) {
      error.value = data.error
    } else if (data.hasUpdate && data.releaseNote) {
      updateAvailable.value = true
      latestVersion.value = data.releaseNote.version
      publishedAt.value = data.releaseNote.publishedAt
      releaseNotes.value = formatMarkdown(data.releaseNote.notes)
      downloadUrl.value = data.releaseNote.pageUrl
    }
  } catch (err) {
    error.value = '检查更新失败，请稍后重试'
    console.error('检查更新失败:', err)
  } finally {
    checking.value = false
  }
}

// 简单的 Markdown 转换
function formatMarkdown(text: string): string {
  if (!text) return ''
  return text
    .replace(/^### (.+)$/gm, '<h4>$1</h4>')
    .replace(/^## (.+)$/gm, '<h3>$1</h3>')
    .replace(/^# (.+)$/gm, '<h2>$1</h2>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/\n/g, '<br>')
}

onMounted(() => {
  loadVersion()
})
</script>

<style scoped>
.upgrade-panel {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
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

.label {
  color: var(--sub, #999);
}

.value {
  color: var(--text, #fff);
  font-weight: 500;
}

.check-btn {
  padding: 12px 24px;
  background: var(--primary, #007bff);
  color: white;
  border: none;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.check-btn:hover:not(:disabled) {
  background: var(--primary-hover, #0056b3);
  transform: translateY(-1px);
}

.check-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.check-btn.loading {
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.update-notice {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border-radius: 12px;
  padding: 20px;
  color: white;
  animation: slideIn 0.3s ease-out;
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.notice-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
  font-size: 18px;
  font-weight: 600;
}

.icon {
  font-size: 24px;
}

.update-details {
  margin-bottom: 16px;
  font-size: 14px;
  line-height: 1.6;
}

.update-details p {
  margin: 8px 0;
}

.release-notes {
  margin-top: 12px;
  padding: 12px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  max-height: 200px;
  overflow-y: auto;
}

.notes-content {
  margin-top: 8px;
  font-size: 13px;
  line-height: 1.5;
}

.download-btn {
  display: inline-block;
  padding: 10px 20px;
  background: white;
  color: #667eea;
  text-decoration: none;
  border-radius: 6px;
  font-weight: 600;
  transition: all 0.2s;
}

.download-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
}

.up-to-date {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px;
  background: var(--card, #1a1a1a);
  border-radius: 8px;
  color: var(--text, #fff);
  animation: slideIn 0.3s ease-out;
}

.error-notice {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px;
  background: #ff4444;
  color: white;
  border-radius: 8px;
  animation: slideIn 0.3s ease-out;
}

@media (max-width: 640px) {
  .upgrade-panel {
    padding: 16px;
  }

  .version-info {
    padding: 12px;
  }

  .info-row {
    font-size: 13px;
  }

  .update-notice {
    padding: 16px;
  }
}
</style>
