// 响应式断点 composable（useXxx 命名）
import { ref, onMounted, onUnmounted } from 'vue'

export function useBreakpoint() {
  const breakpoints = { xs: 480, sm: 640, md: 768, lg: 1024, xl: 1280, '2xl': 1536 }
  const current = ref('xl')
  const isMobile = ref(false)
  const isTablet = ref(false)
  const isDesktop = ref(true)

  function update() {
    const w = window.innerWidth
    if (w < breakpoints.sm) {
      current.value = w < breakpoints.xs ? 'xs' : 'sm'
    } else if (w < breakpoints.lg) {
      current.value = w < breakpoints.md ? 'md' : 'lg'
    } else {
      current.value = w < breakpoints.xl ? 'xl' : '2xl'
    }
    isMobile.value = w < breakpoints.md
    isTablet.value = w >= breakpoints.md && w < breakpoints.lg
    isDesktop.value = w >= breakpoints.lg
  }

  onMounted(() => {
    update()
    window.addEventListener('resize', update)
  })

  onUnmounted(() => {
    window.removeEventListener('resize', update)
  })

  return { current, isMobile, isTablet, isDesktop }
}
