<template>
  <el-container class="shell">
    <el-header class="header">
      <div class="brand" @click="$router.push('/')">
        <Logo />
        <div>
          <div class="title">MockFlow</div>
          <div class="sub">基于时间的 Mock API</div>
        </div>
      </div>
      <el-select
        v-model="currentId"
        class="project-select"
        placeholder="选择项目"
        filterable
        @change="onProjectChange"
      >
        <el-option
          v-for="p in projects"
          :key="p.id"
          :label="`${p.name}  :${p.port}`"
          :value="p.id"
        />
      </el-select>
      <el-menu mode="horizontal" :ellipsis="false" :router="true" :default-active="active">
        <el-menu-item index="/">接口列表</el-menu-item>
        <el-menu-item index="/endpoints/new">新建接口</el-menu-item>
        <el-menu-item index="/test">调用测试</el-menu-item>
        <el-menu-item index="/projects">项目管理</el-menu-item>
      </el-menu>
    </el-header>
    <el-main>
      <router-view :key="currentId || 'none'" />
    </el-main>
  </el-container>
</template>

<script setup>
import { computed, onMounted, provide, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import Logo from './components/Logo.vue'
import { listProjects } from './api'

const KEY = 'mockflow.projectId'
const route = useRoute()
const router = useRouter()
const projects = ref([])
const currentId = ref(null)

const currentProject = computed(() => projects.value.find((p) => p.id === currentId.value) || null)
provide('projects', projects)
provide('currentProject', currentProject)
provide('reloadProjects', loadProjects)

const active = computed(() => {
  if (route.path.startsWith('/test')) return '/test'
  if (route.path.startsWith('/projects')) return '/projects'
  if (route.path.startsWith('/endpoints/new')) return '/endpoints/new'
  return '/'
})

async function loadProjects() {
  try {
    projects.value = await listProjects()
    const saved = Number(localStorage.getItem(KEY))
    if (projects.value.some((p) => p.id === saved)) {
      currentId.value = saved
    } else if (projects.value[0]) {
      currentId.value = projects.value[0].id
      localStorage.setItem(KEY, String(currentId.value))
    } else {
      currentId.value = null
    }
  } catch (e) {
    ElMessage.error(e.message)
  }
}

function onProjectChange(id) {
  localStorage.setItem(KEY, String(id))
  if (route.path.startsWith('/endpoints/') && route.path !== '/endpoints/new') {
    router.push('/')
  }
}

onMounted(loadProjects)
</script>

<style scoped>
.shell {
  min-height: 100vh;
}
.header {
  display: flex;
  align-items: center;
  gap: 16px;
  background: #fff;
  border-bottom: 1px solid #ebeef5;
  height: 64px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  min-width: 180px;
}
.title {
  font-weight: 700;
  line-height: 1.2;
}
.sub {
  font-size: 12px;
  color: #909399;
}
.project-select {
  width: 220px;
}
.el-menu {
  flex: 1;
  border-bottom: none;
}
</style>
