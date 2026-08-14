<script setup lang="ts">
// 根组件：仅提供路由出口 + 全局 Element Plus 中文语言包。
// 按需引入后不再 app.use(ElementPlus)，通过 ElConfigProvider 注入 zh-cn locale。
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>

<template>
  <el-config-provider :locale="zhCn">
    <router-view />
  </el-config-provider>
</template>

<style scoped>
:global(#app) {
  height: 100%;
}
</style>
