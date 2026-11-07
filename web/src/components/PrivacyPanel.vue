<script setup lang="ts">
const emit = defineEmits<{ (e: 'agree'): void; (e: 'disagree'): void }>()

const policy = `欢迎使用夜帧影视。

本应用不含任何影视内容，仅提供界面、协议实现与播放器。看什么完全取决于你导入的 TVBox 配置（JSON）。

隐私说明：
1. 本应用不收集、不上传任何个人信息。
2. 你的配置、播放历史、收藏等数据只保存在本机（浏览器本地），不会发送到任何服务器。
3. 应用只在你主动填写时访问网络（拉取站源、直播源、播放地址）。
4. 无广告、无账号、无追踪。

隐私政策全文见：
https://s3a.abber.cn:88/public/privacy.html`
</script>

<template>
  <div class="consent">
    <div class="consent-card">
      <div class="consent-logo">
        <img src="/icons/app_icon.png" alt="夜帧影视" />
      </div>
      <h1 class="consent-title">夜帧影视</h1>
      <div class="consent-body">
        <pre class="consent-text">{{ policy }}</pre>
      </div>
      <div class="consent-actions">
        <el-button size="large" @click="emit('disagree')">不同意并退出</el-button>
        <el-button size="large" type="primary" @click="emit('agree')">同意并继续</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.consent {
  position: fixed;
  inset: 0;
  background: var(--bg);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 5000;
}
.consent-card {
  width: 560px;
  max-width: 92vw;
  background: var(--surface);
  border-radius: 20px;
  padding: 32px;
  display: flex;
  flex-direction: column;
  align-items: center;
}
.consent-logo img {
  width: 72px;
  height: 72px;
  border-radius: 18px;
}
.consent-title {
  margin: 14px 0 18px;
  font-size: 22px;
}
.consent-body {
  width: 100%;
  max-height: 46vh;
  overflow-y: auto;
  background: var(--surface-2);
  border-radius: 12px;
  padding: 16px;
  margin-bottom: 20px;
}
.consent-text {
  margin: 0;
  white-space: pre-wrap;
  font-family: inherit;
  font-size: 13px;
  line-height: 1.7;
  color: var(--sub);
}
.consent-actions {
  display: flex;
  gap: 12px;
}
</style>
