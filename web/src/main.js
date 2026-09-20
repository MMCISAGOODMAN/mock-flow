import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import EndpointList from './views/EndpointList.vue'
import EndpointEdit from './views/EndpointEdit.vue'
import TestPanel from './views/TestPanel.vue'
import ProjectList from './views/ProjectList.vue'
import './style.css'

const router = createRouter({
  history: createWebHistory('/_ui/'),
  routes: [
    { path: '/', component: EndpointList },
    { path: '/endpoints/new', component: EndpointEdit },
    { path: '/endpoints/:id', component: EndpointEdit },
    { path: '/test', component: TestPanel },
    { path: '/projects', component: ProjectList }
  ]
})

createApp(App).use(router).use(ElementPlus).mount('#app')
