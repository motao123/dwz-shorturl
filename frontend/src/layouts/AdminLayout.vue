<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import {
  Odometer,
  Link,
  User,
  UserFilled,
  Stamp,
  TrendCharts,
  Monitor,
  Setting,
  Document,
  Key,
  Connection,
  WarningFilled,
  BellFilled,
  Expand,
  Fold,
  ArrowDown,
  SwitchButton,
  Sunny,
  Moon
} from '@element-plus/icons-vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { hasPermission } from '@/utils/permission'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const themeStore = useThemeStore()

// 窄屏（≤992px）自动折叠侧栏，宽屏时恢复展开（仅当用户此前未手动折叠）
let mediaQuery: MediaQueryList | null = null
function handleNarrowChange(e: MediaQueryListEvent | MediaQueryList) {
  if (e.matches && !appStore.sidebarCollapsed) {
    appStore.setSidebarCollapsed(true)
  } else if (!e.matches && appStore.sidebarCollapsed && !appStore.userCollapsed) {
    appStore.setSidebarCollapsed(false)
  }
}
onMounted(() => {
  mediaQuery = window.matchMedia('(max-width: 992px)')
  handleNarrowChange(mediaQuery)
  mediaQuery.addEventListener?.('change', handleNarrowChange)
})
onBeforeUnmount(() => {
  mediaQuery?.removeEventListener?.('change', handleNarrowChange)
})

interface MenuEntry {
  path: string
  title: string
  icon: typeof Odometer
  perm?: string
}

const menuEntries: MenuEntry[] = [
  { path: '/dashboard', title: '仪表盘', icon: Odometer, perm: 'stats.read' },
  { path: '/short-urls', title: '短链管理', icon: Link, perm: 'short_urls.read' },
  { path: '/domains', title: '域名管理', icon: Connection, perm: 'domains.read' },
  { path: '/stats', title: '统计分析', icon: TrendCharts, perm: 'stats.read' },
  { path: '/monitor', title: '系统监控', icon: Monitor, perm: 'stats.read' },
  { path: '/users', title: '用户管理', icon: User, perm: 'users.read' },
  { path: '/members', title: '注册用户', icon: UserFilled, perm: 'users.read' },
  { path: '/roles', title: '角色管理', icon: Stamp, perm: 'roles.read' },
  { path: '/configs', title: '系统配置', icon: Setting, perm: 'configs.read' },
  { path: '/audit-logs', title: '审计日志', icon: Document, perm: 'audit.read' },
  { path: '/violations', title: '违规审核', icon: WarningFilled, perm: 'audit.read' },
  { path: '/api-keys', title: 'API 密钥', icon: Key, perm: 'api_keys.read' },
  { path: '/api-docs', title: 'API 文档', icon: Document, perm: 'api_keys.read' },
  { path: '/webhooks', title: 'Webhook', icon: BellFilled, perm: 'api_keys.read' }
]

const visibleMenus = computed(() =>
  menuEntries.filter((m) => !m.perm || hasPermission(m.perm))
)

const activeMenu = computed(() => '/' + (route.path.split('/')[1] || 'dashboard'))

const crumbs = computed(() => {
  const current = menuEntries.find((m) => m.path === activeMenu.value)
  return [
    { title: '首页', path: '/dashboard' },
    ...(current ? [{ title: current.title, path: current.path }] : [])
  ]
})

watch(
  crumbs,
  (val) => appStore.setBreadcrumbs(val),
  { immediate: true }
)

const roleLabel = computed(() => {
  const map: Record<string, string> = {
    super_admin: '超级管理员',
    admin: '管理员',
    operator: '运营',
    viewer: '只读'
  }
  const first = authStore.roles[0]
  return first ? map[first] ?? first : '—'
})

async function handleLogout() {
  try {
    await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '退出登录',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }
  await authStore.logout()
  ElMessage.success('已退出登录')
  router.push('/login')
}
</script>

<template>
  <el-container class="layout">
    <!-- 侧栏 -->
    <el-aside :width="`${appStore.sidebarWidth}px`" class="layout__aside">
      <div class="brand" :class="{ 'brand--mini': appStore.sidebarCollapsed }">
        <div class="brand__mark">
          <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true">
            <path
              d="M10.5 13.5a3.5 3.5 0 0 1 0-5l2-2a3.54 3.54 0 0 1 5 5l-1.2 1.2"
              stroke="#f5a623" stroke-width="2.2" fill="none" stroke-linecap="round"
            />
            <path
              d="M13.5 10.5a3.5 3.5 0 0 1 0 5l-2 2a3.54 3.54 0 0 1-5-5l1.2-1.2"
              stroke="#e8f4f2" stroke-width="2.2" fill="none" stroke-linecap="round"
            />
          </svg>
        </div>
        <transition name="brand-text">
          <div v-show="!appStore.sidebarCollapsed" class="brand__text">
            <span class="brand__name">DWZ 控制台</span>
            <span class="brand__sub mono">SHORTURL CONSOLE</span>
          </div>
        </transition>
      </div>

      <el-menu
        :default-active="activeMenu"
        :collapse="appStore.sidebarCollapsed"
        :collapse-transition="false"
        router
        class="layout__menu"
        background-color="transparent"
        text-color="#9db8bd"
        active-text-color="#ffffff"
      >
        <el-menu-item v-for="item in visibleMenus" :key="item.path" :index="item.path">
          <el-icon><component :is="item.icon" /></el-icon>
          <template #title><span>{{ item.title }}</span></template>
        </el-menu-item>
      </el-menu>

      <div v-show="!appStore.sidebarCollapsed" class="aside-foot mono">
        <span class="aside-foot__dot"></span> v1.0 · 后台服务运行中
      </div>
    </el-aside>

    <el-container class="layout__body">
      <!-- 顶栏 -->
      <el-header class="layout__header" height="58px">
        <div class="header-left">
          <button class="icon-btn" aria-label="切换侧栏" :title="appStore.sidebarCollapsed ? '展开菜单' : '收起菜单'" @click="appStore.toggleSidebar()">
            <el-icon :size="17"><component :is="appStore.sidebarCollapsed ? Expand : Fold" /></el-icon>
          </button>
          <el-breadcrumb separator="/" class="crumbs">
            <el-breadcrumb-item v-for="(c, i) in crumbs" :key="c.path ?? i" :to="c.path ? { path: c.path } : undefined">
              {{ c.title }}
            </el-breadcrumb-item>
          </el-breadcrumb>
        </div>

        <div class="header-right">
          <el-dropdown trigger="click" @command="(cmd: string) => cmd === 'logout' && handleLogout()">
            <div class="user-chip">
              <span class="user-chip__avatar">{{ authStore.displayName.slice(0, 1).toUpperCase() }}</span>
              <span class="user-chip__meta">
                <span class="user-chip__name">{{ authStore.displayName }}</span>
                <span class="user-chip__role mono">{{ roleLabel }}</span>
              </span>
              <el-icon class="user-chip__caret"><ArrowDown /></el-icon>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>
                  <span class="mono" style="font-size: 12px">@{{ authStore.userInfo?.username }}</span>
                </el-dropdown-item>
                <el-dropdown-item divided command="logout">
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>

          <button class="icon-btn" aria-label="切换主题" :title="themeStore.dark ? '切换到浅色' : '切换到深色'" @click="themeStore.toggle()">
            <el-icon :size="17"><component :is="themeStore.dark ? Sunny : Moon" /></el-icon>
          </button>
        </div>
      </el-header>

      <!-- 内容区 -->
      <el-main class="layout__main">
        <router-view v-slot="{ Component }">
          <transition name="dwz-fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.layout {
  height: 100vh;
}

/* ---------------- 侧栏 ---------------- */

.layout__aside {
  display: flex;
  flex-direction: column;
  background: linear-gradient(178deg, #0e3138 0%, #0a2227 70%, #081c20 100%);
  transition: width 0.28s cubic-bezier(0.22, 1, 0.36, 1);
  overflow: hidden;
  border-right: 1px solid rgba(255, 255, 255, 0.04);
}

.brand {
  display: flex;
  align-items: center;
  gap: 11px;
  height: 58px;
  padding: 0 16px;
  flex-shrink: 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.brand--mini {
  justify-content: center;
  padding: 0;
}

.brand__mark {
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  display: grid;
  place-items: center;
  border-radius: 9px;
  background: rgba(245, 166, 35, 0.12);
  border: 1px solid rgba(245, 166, 35, 0.28);
}

.brand__text {
  display: flex;
  flex-direction: column;
  line-height: 1.25;
  white-space: nowrap;
}

.brand__name {
  color: #f2faf8;
  font-weight: 800;
  font-size: 15px;
  letter-spacing: 0.01em;
}

.brand__sub {
  font-size: 9px;
  letter-spacing: 0.22em;
  color: #5e848b;
}

.brand-text-enter-active,
.brand-text-leave-active {
  transition: opacity 0.18s ease;
}
.brand-text-enter-from,
.brand-text-leave-to {
  opacity: 0;
}

.layout__menu {
  flex: 1;
  border-right: none !important;
  padding: 12px 8px;
  overflow-y: auto;
  overflow-x: hidden;
  --el-menu-item-height: 44px;
}

.layout__menu :deep(.el-menu-item) {
  border-radius: 9px;
  margin-bottom: 3px;
  position: relative;
  font-size: 14px;
  transition: background-color 0.18s ease, color 0.18s ease;
}

.layout__menu :deep(.el-menu-item:hover) {
  background: rgba(255, 255, 255, 0.06) !important;
  color: #e8f4f2 !important;
}

.layout__menu :deep(.el-menu-item.is-active) {
  background: rgba(245, 166, 35, 0.13) !important;
  color: #ffffff !important;
  font-weight: 700;
}

.layout__menu :deep(.el-menu-item.is-active::before) {
  content: '';
  position: absolute;
  left: -8px;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: 0 3px 3px 0;
  background: var(--dwz-amber);
  box-shadow: 0 0 10px rgba(245, 166, 35, 0.7);
}

.aside-foot {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 14px 18px;
  font-size: 10.5px;
  color: #4f747b;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
  white-space: nowrap;
}

.aside-foot__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #2dd4a7;
  box-shadow: 0 0 6px #2dd4a7;
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}

/* ---------------- 顶栏 ---------------- */

.layout__body {
  min-width: 0;
}

.layout__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: color-mix(in srgb, var(--el-bg-color, #fff) 92%, transparent);
  backdrop-filter: blur(6px);
  border-bottom: 1px solid var(--dwz-line);
  padding: 0 22px;
  position: sticky;
  top: 0;
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.icon-btn {
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border: 1px solid var(--dwz-line);
  border-radius: 8px;
  background: var(--el-bg-color, #fff);
  color: var(--dwz-text);
  cursor: pointer;
  transition: all 0.16s ease;
}

.icon-btn:hover {
  border-color: var(--dwz-petrol);
  color: var(--dwz-petrol);
  box-shadow: 0 2px 6px rgba(14, 110, 117, 0.12);
}

/* 键盘可访问性：图标按钮焦点可见环 */
.icon-btn:focus-visible {
  outline: 2px solid var(--dwz-petrol);
  outline-offset: 2px;
}

.crumbs :deep(.el-breadcrumb__inner) {
  font-weight: 500;
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 10px 5px 5px;
  border-radius: 999px;
  cursor: pointer;
  transition: background-color 0.16s ease;
  outline: none;
}

.user-chip:hover {
  background: var(--el-fill-color-light, #edf3f4);
}

.user-chip__avatar {
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border-radius: 50%;
  background: linear-gradient(135deg, #0e6e75, #0a4a50);
  color: #fff;
  font-weight: 800;
  font-size: 15px;
  box-shadow: 0 2px 6px rgba(14, 110, 117, 0.35);
}

.user-chip__meta {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}

.user-chip__name {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--dwz-ink);
}

.user-chip__role {
  font-size: 10px;
  letter-spacing: 0.08em;
  color: var(--dwz-amber-deep);
}

.user-chip__caret {
  color: var(--dwz-text-dim);
  font-size: 12px;
}

/* ---------------- 内容区 ---------------- */

.layout__main {
  padding: 0;
  background-color: var(--dwz-paper);
  background-image: radial-gradient(rgba(14, 110, 117, 0.055) 1px, transparent 1px);
  background-size: 22px 22px;
  overflow-y: auto;
  height: calc(100vh - 58px);
}

/* ---------------- 响应式 ---------------- */

@media (max-width: 992px) {
  .layout__header {
    padding: 0 12px;
  }

  .crumbs {
    display: none;
  }

  .user-chip__meta {
    display: none;
  }

  .user-chip {
    padding: 4px;
  }

  .user-chip__caret {
    display: none;
  }
}

@media (max-width: 640px) {
  .brand__sub {
    display: none;
  }
}
</style>
