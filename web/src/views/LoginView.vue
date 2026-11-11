<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { authState, login } from '@/auth'

const router = useRouter()
const route = useRoute()

const user = ref('')
const pass = ref('')
const showPass = ref(false)
const loading = ref(false)
const errMsg = ref('')
const shake = ref(false)
const userEl = ref<HTMLInputElement>()
const passEl = ref<HTMLInputElement>()

const canSubmit = computed(() => user.value.trim() !== '' && pass.value !== '' && !loading.value)

onMounted(async () => {
  // 记住上次登录的用户名（只记名字，不记密码）
  try {
    user.value = localStorage.getItem('yz_login_user') || ''
  } catch {
    /* 隐私模式忽略 */
  }
  await nextTick()
  // 有用户名就直接把光标放到密码框，少一次点击
  ;(user.value ? passEl.value : userEl.value)?.focus()
})

function fail(msg: string) {
  errMsg.value = msg
  shake.value = true
  window.setTimeout(() => (shake.value = false), 420)
}

async function submit() {
  if (!canSubmit.value) return
  errMsg.value = ''
  loading.value = true
  try {
    await login(user.value.trim(), pass.value)
    try {
      localStorage.setItem('yz_login_user', user.value.trim())
    } catch {
      /* ignore */
    }
    const redirect = (route.query.redirect as string) || '/'
    // 只接受站内路径，避免被 ?redirect= 带到外部站点
    router.replace(redirect.startsWith('/') && !redirect.startsWith('//') ? redirect : '/')
  } catch (e: any) {
    fail(e?.message || '登录失败，请重试')
    pass.value = ''
    await nextTick()
    passEl.value?.focus()
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <!-- 背景：两团缓慢漂移的光晕 + 极细网格，营造纵深但不喧宾夺主 -->
    <div class="aurora" aria-hidden="true">
      <span class="blob b1"></span>
      <span class="blob b2"></span>
      <span class="grid"></span>
      <span class="vignette"></span>
    </div>

    <main class="card" :class="{ shake }">
      <header class="brand">
        <div class="mark" aria-hidden="true">
          <svg viewBox="0 0 48 48" width="30" height="30" fill="none">
            <rect x="5" y="9" width="30" height="21" rx="4" stroke="currentColor" stroke-width="2.2" />
            <path d="M38 17l5.5-3.5v21L38 31" stroke="currentColor" stroke-width="2.2"
                  stroke-linecap="round" stroke-linejoin="round" />
            <path d="M13 38h14" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" />
          </svg>
        </div>
        <h1>夜帧影视</h1>
        <p class="sub">登录后继续观看</p>
      </header>

      <form class="form" @submit.prevent="submit">
        <label class="field">
          <span class="label">账号</span>
          <input
            ref="userEl"
            v-model="user"
            type="text"
            autocomplete="username"
            spellcheck="false"
            placeholder="请输入账号"
            :disabled="loading"
          />
        </label>

        <label class="field">
          <span class="label">密码</span>
          <div class="pass-wrap">
            <input
              ref="passEl"
              v-model="pass"
              :type="showPass ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="请输入密码"
              :disabled="loading"
            />
            <button
              type="button"
              class="eye"
              :aria-label="showPass ? '隐藏密码' : '显示密码'"
              :title="showPass ? '隐藏密码' : '显示密码'"
              @click="showPass = !showPass"
            >
              <!-- 睁眼 / 闭眼，纯内联 SVG，跟随主题色 -->
              <svg v-if="!showPass" viewBox="0 0 24 24" width="18" height="18" fill="none"
                   stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M2 12s3.6-6.5 10-6.5S22 12 22 12s-3.6 6.5-10 6.5S2 12 2 12z" />
                <circle cx="12" cy="12" r="2.8" />
              </svg>
              <svg v-else viewBox="0 0 24 24" width="18" height="18" fill="none"
                   stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M3 3l18 18" />
                <path d="M10.6 6.1A9.6 9.6 0 0112 6c6.4 0 10 6 10 6a17 17 0 01-3.2 3.8" />
                <path d="M6.4 7.9C3.8 9.6 2 12 2 12s3.6 6 10 6a9.9 9.9 0 003.6-.7" />
              </svg>
            </button>
          </div>
        </label>

        <transition name="err">
          <p v-if="errMsg" class="err" role="alert">{{ errMsg }}</p>
        </transition>

        <button class="submit" type="submit" :disabled="!canSubmit">
          <span v-if="!loading">进入</span>
          <span v-else class="dots" aria-label="登录中">
            <i></i><i></i><i></i>
          </span>
        </button>
      </form>

      <footer class="foot">
        <span>{{ authState.needLogin ? '需要账号才能访问此服务器' : '' }}</span>
      </footer>
    </main>

    <p class="copyright">夜帧影视 · Yezhen</p>
  </div>
</template>

<style scoped>
.login-page {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--bg);
  color: var(--txt);
  overflow: hidden;
}

/* ===== 背景光晕 ===== */
.aurora {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}
.blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  opacity: 0.5;
}
/* 主色取自主题的 --live（红），副色用 --selected（黄），
   亮色主题下会自动跟着令牌变浅 */
.b1 {
  width: 46vw;
  height: 46vw;
  min-width: 320px;
  min-height: 320px;
  top: -12vw;
  left: -8vw;
  background: radial-gradient(circle at 30% 30%, var(--live), transparent 70%);
  animation: float1 18s ease-in-out infinite;
}
.b2 {
  width: 40vw;
  height: 40vw;
  min-width: 280px;
  min-height: 280px;
  right: -10vw;
  bottom: -12vw;
  background: radial-gradient(circle at 70% 70%, var(--selected), transparent 72%);
  opacity: 0.22;
  animation: float2 22s ease-in-out infinite;
}
.grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--white-15) 1px, transparent 1px),
    linear-gradient(90deg, var(--white-15) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: radial-gradient(ellipse at 50% 45%, rgba(0, 0, 0, 0.55), transparent 72%);
  -webkit-mask-image: radial-gradient(ellipse at 50% 45%, rgba(0, 0, 0, 0.55), transparent 72%);
}
.vignette {
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at 50% 40%, transparent 30%, var(--bg) 95%);
}
@keyframes float1 {
  0%, 100% { transform: translate3d(0, 0, 0) scale(1); }
  50% { transform: translate3d(4vw, 3vh, 0) scale(1.08); }
}
@keyframes float2 {
  0%, 100% { transform: translate3d(0, 0, 0) scale(1); }
  50% { transform: translate3d(-3vw, -4vh, 0) scale(1.1); }
}

/* ===== 卡片 ===== */
.card {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 380px;
  padding: 40px 34px 26px;
  border-radius: 20px;
  border: 1px solid var(--border);
  background: var(--surface); /* 兜底：不支持 color-mix 的浏览器 */
  background: color-mix(in srgb, var(--surface) 82%, transparent);
  backdrop-filter: blur(22px) saturate(1.2);
  -webkit-backdrop-filter: blur(22px) saturate(1.2);
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.06) inset,
    0 24px 70px rgba(0, 0, 0, 0.45);
}
/* 顶部一道极细高光，卡片「浮起来」的关键 */
.card::before {
  content: '';
  position: absolute;
  inset: 0 22% auto 22%;
  height: 1px;
  background: linear-gradient(90deg, transparent, var(--white-30), transparent);
}
.shake { animation: shake 0.4s cubic-bezier(0.36, 0.07, 0.19, 0.97); }
@keyframes shake {
  10%, 90% { transform: translateX(-1px); }
  20%, 80% { transform: translateX(3px); }
  30%, 50%, 70% { transform: translateX(-5px); }
  40%, 60% { transform: translateX(5px); }
}

/* ===== 品牌 ===== */
.brand { text-align: center; margin-bottom: 30px; }
.mark {
  width: 54px;
  height: 54px;
  margin: 0 auto 16px;
  display: grid;
  place-items: center;
  border-radius: 16px;
  color: var(--selected);
  background: var(--card);
  border: 1px solid var(--border);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.28);
}
.brand h1 {
  margin: 0;
  font-size: 21px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-indent: 0.14em; /* 抵消字距带来的视觉偏左 */
}
.sub {
  margin: 8px 0 0;
  font-size: 12.5px;
  letter-spacing: 0.06em;
  color: var(--dim);
}

/* ===== 表单 ===== */
.form { display: flex; flex-direction: column; gap: 16px; }
.field { display: flex; flex-direction: column; gap: 7px; }
.label {
  font-size: 12px;
  letter-spacing: 0.08em;
  color: var(--sub);
}
.field input {
  width: 100%;
  height: 44px;
  padding: 0 14px;
  box-sizing: border-box;
  border-radius: 10px;
  border: 1px solid var(--border);
  background: var(--card);
  color: var(--txt);
  font-size: 14px;
  outline: none;
  transition: border-color 0.18s, background 0.18s, box-shadow 0.18s;
}
.field input::placeholder { color: var(--dim); }
.field input:hover:not(:disabled) { border-color: var(--white-30); }
.field input:focus {
  border-color: var(--selected);
  background: var(--surface-2);
  box-shadow: 0 0 0 3px var(--white-15);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--selected) 18%, transparent);
}
.field input:disabled { opacity: 0.6; cursor: not-allowed; }

.pass-wrap { position: relative; }
.pass-wrap input { padding-right: 44px; }
.eye {
  position: absolute;
  top: 50%;
  right: 6px;
  transform: translateY(-50%);
  width: 32px;
  height: 32px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--dim);
  cursor: pointer;
  transition: color 0.15s, background 0.15s;
}
.eye:hover { color: var(--txt); background: var(--white-15); }

.err {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--warn);
  padding-left: 2px;
}
.err-enter-active, .err-leave-active { transition: opacity 0.18s, transform 0.18s; }
.err-enter-from, .err-leave-to { opacity: 0; transform: translateY(-4px); }

.submit {
  position: relative;
  height: 46px;
  margin-top: 6px;
  border: none;
  border-radius: 10px;
  background: var(--selected);
  color: #141218;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.16em;
  text-indent: 0.16em;
  cursor: pointer;
  transition: transform 0.14s, box-shadow 0.2s, filter 0.2s;
  box-shadow: 0 10px 26px rgba(0, 0, 0, 0.3);
  box-shadow: 0 10px 26px color-mix(in srgb, var(--selected) 22%, transparent);
}
.submit:hover:not(:disabled) { filter: brightness(1.06); transform: translateY(-1px); }
.submit:active:not(:disabled) { transform: translateY(0) scale(0.995); }
.submit:disabled { opacity: 0.5; cursor: not-allowed; box-shadow: none; }

.dots { display: inline-flex; gap: 6px; }
.dots i {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: #141218;
  animation: bounce 1s infinite ease-in-out;
}
.dots i:nth-child(2) { animation-delay: 0.15s; }
.dots i:nth-child(3) { animation-delay: 0.3s; }
@keyframes bounce {
  0%, 80%, 100% { opacity: 0.35; transform: translateY(0); }
  40% { opacity: 1; transform: translateY(-4px); }
}

.foot {
  margin-top: 22px;
  text-align: center;
  font-size: 12px;
  color: var(--dim);
  min-height: 16px;
}
.copyright {
  position: relative;
  z-index: 1;
  margin: 26px 0 0;
  font-size: 11.5px;
  letter-spacing: 0.18em;
  color: var(--dim);
  opacity: 0.7;
}

/* ===== 移动端 ===== */
@media (max-width: 640px) {
  .login-page { padding: 20px 18px; justify-content: flex-start; padding-top: 12vh; }
  .card { max-width: 100%; padding: 32px 22px 22px; border-radius: 18px; }
  .brand { margin-bottom: 24px; }
}

/* 尊重系统的「减少动态效果」设置 */
@media (prefers-reduced-motion: reduce) {
  .b1, .b2 { animation: none; }
  .shake { animation: none; }
  .dots i { animation: none; opacity: 0.8; }
}
</style>
