<template>
  <div class="login-root">
    <!-- 背景装饰 -->
    <div class="login-bg">
      <div class="bg-gradient" />
      <div class="bg-pattern" />
    </div>

    <!-- 登录卡片 -->
    <div class="login-card-wrapper">
      <div class="login-card">
        <!-- 品牌区 -->
        <div class="login-brand">
          <div class="brand-icon flex items-center justify-center rounded-2xl">
            <el-icon :size="28" class="brand-icon-inner"><Monitor /></el-icon>
          </div>
          <div class="brand-text">
            <h1 class="brand-title">Ydsz Trace</h1>
            <p class="brand-subtitle">分布式日志追踪检索控制台</p>
          </div>
        </div>

        <!-- 表单区 -->
        <el-form :model="form" label-position="top" @submit.prevent="onSubmit" class="login-form">
          <el-form-item>
            <template #label><span class="form-label">用户名</span></template>
            <el-input
              v-model="form.username"
              size="large"
              placeholder="请输入管理员用户名"
              :prefix-icon="User"
              class="login-input"
            />
          </el-form-item>
          <el-form-item>
            <template #label><span class="form-label">密码</span></template>
            <el-input
              v-model="form.password"
              type="password"
              size="large"
              show-password
              placeholder="请输入密码"
              :prefix-icon="Lock"
              @keyup.enter="onSubmit"
              class="login-input"
            />
          </el-form-item>

          <el-button
            type="primary"
            size="large"
            class="login-btn w-full"
            :loading="loading"
            @click="onSubmit"
          >
            登&nbsp;&nbsp;录
          </el-button>
        </el-form>

        <!-- 底部信息 -->
        <div class="login-footer">
          <p class="footer-tip">AI 驱动的日志排障定位平台 v2.0</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
/**
 * 登录页 v2.0 — 品牌感增强版。
 *
 * 采用左右分栏布局：左侧品牌展示区 + 右侧登录表单。
 * 背景使用渐变 + 几何装饰。
 * 登录成功后写 token，按 router query redirect 回跳。
 */
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { User, Lock, Monitor } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { login } from '@/api/auth'
import { useAuth } from '@/store/auth'

const form = ref({ username: '', password: '' })
const loading = ref(false)
const auth = useAuth()
const route = useRoute()
const router = useRouter()

async function onSubmit() {
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    const { data } = await login(form.value.username, form.value.password)
    if (data && data.code === '200') {
      auth.setUser(data.data?.username || form.value.username)
      ElMessage.success(data.msg || '登录成功')
      router.push(route.query.redirect || '/conversation')
    } else {
      ElMessage.error(data?.msg || '登录失败')
    }
  } catch {
    ElMessage.error('登录请求失败，请确认服务端已启动且可访问')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-root {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

/* ===== 背景 ===== */
.login-bg {
  position: absolute;
  inset: 0;
  z-index: 0;
}
.bg-gradient {
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, #0f172a 0%, #1e3a5f 40%, #1e40af 70%, #1d4ed8 100%);
}
.bg-pattern {
  position: absolute;
  inset: 0;
  background-image:
    radial-gradient(circle at 20% 30%, rgba(96, 165, 250, 0.12) 0%, transparent 50%),
    radial-gradient(circle at 80% 70%, rgba(37, 99, 235, 0.15) 0%, transparent 50%),
    radial-gradient(circle at 60% 20%, rgba(147, 197, 253, 0.08) 0%, transparent 40%);
}

/* ===== 卡片 ===== */
.login-card-wrapper {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 420px;
  padding: 20px;
}
.login-card {
  background: rgba(255, 255, 255, 0.97);
  border-radius: 20px;
  padding: 40px 36px 32px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.15),
    0 0 0 1px rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(8px);
}

/* ===== 品牌区 ===== */
.login-brand {
  text-align: center;
  margin-bottom: 36px;
}
.brand-icon {
  width: 56px;
  height: 56px;
  margin: 0 auto 16px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  box-shadow: 0 4px 14px rgba(37, 99, 235, 0.35);
}
.brand-icon-inner {
  color: white;
}
.brand-title {
  font-size: 24px;
  font-weight: 700;
  color: #0f172a;
  margin: 0 0 6px;
  letter-spacing: -0.02em;
}
.brand-subtitle {
  font-size: 13.5px;
  color: #64748b;
  margin: 0;
}

/* ===== 表单 ===== */
.login-form :deep(.el-form-item__label) {
  padding-bottom: 4px;
}
.form-label {
  font-size: 13px;
  font-weight: 600;
  color: #334155;
}
.login-input :deep(.el-input__wrapper) {
  border-radius: 10px !important;
  padding: 4px 14px !important;
  box-shadow: 0 0 0 1px #e2e8f0 inset !important;
}
.login-input :deep(.el-input__wrapper:hover) {
  box-shadow: 0 0 0 1px #94a3b8 inset !important;
}
.login-input :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 2px #2563eb inset !important;
}

/* ===== 按钮 ===== */
.login-btn {
  height: 44px !important;
  border-radius: 10px !important;
  font-size: 15px !important;
  font-weight: 600 !important;
  letter-spacing: 0.08em;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%) !important;
  border: none !important;
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3);
  transition: all 0.2s;
  margin-top: 8px;
}
.login-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 18px rgba(37, 99, 235, 0.4);
}
.login-btn:active {
  transform: translateY(0);
}

/* ===== 底部 ===== */
.login-footer {
  margin-top: 24px;
  text-align: center;
}
.footer-tip {
  font-size: 12px;
  color: #94a3b8;
  margin: 0;
}
</style>
