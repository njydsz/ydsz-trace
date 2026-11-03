<template>
  <div :class="{ 'pb-16': isMobile }">
    <router-view />
    <MobileNav v-if="isMobile" />
  </div>
</template>

<script setup>
import { useBreakpoint } from '@/composables/useBreakpoint'
import MobileNav from '@/components/MobileNav.vue'

const { isMobile } = useBreakpoint()
</script>
