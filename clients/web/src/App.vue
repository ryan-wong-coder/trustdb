<script setup lang="ts">
import { useRoute } from 'vue-router'
import Sidebar from '@/components/Sidebar.vue'
import TopBar from '@/components/TopBar.vue'

const route = useRoute()
</script>

<template>
  <div v-if="route.name === 'login'" class="h-full w-full">
    <router-view />
  </div>
  <div v-else class="app-shell h-full w-full flex overflow-hidden">
    <div class="ambient-orb one"></div>
    <div class="ambient-orb two"></div>
    <Sidebar />
    <main class="relative z-[1] flex-1 h-full flex flex-col min-w-0">
      <TopBar />
      <div class="flex-1 overflow-y-auto px-7 py-6 min-w-0">
        <router-view v-slot="{ Component, route: r }">
          <transition name="page" mode="out-in">
            <component :is="Component" :key="r.fullPath" />
          </transition>
        </router-view>
      </div>
    </main>
  </div>
</template>

<style>
.page-enter-active, .page-leave-active { transition: all .24s cubic-bezier(.2,.9,.2,1); }
.page-enter-from { opacity: 0; transform: translateY(14px) scale(.985); }
.page-leave-to   { opacity: 0; transform: translateY(-8px) scale(.99); }
</style>
