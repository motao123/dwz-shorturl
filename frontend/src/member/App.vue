<script setup lang="ts">
// 会员端根组件：注入 Element Plus 中文语言包（按需引入后不再全量 app.use）。
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>

<template>
  <el-config-provider :locale="zhCn">
    <router-view />
  </el-config-provider>
</template>

<style>
body {
  margin: 0;
  background: #f6f8f9;
  font-family: system-ui, -apple-system, "Segoe UI", "PingFang SC", sans-serif;
}
</style>
