// 路由 query 参数消费 composable。
//
// 背景（V2.1-07）：GlobalSearchDialog 通过 router.push({ query }) 跳转时，
// 目标视图此前不读取 query，导致跳转后落在空白表单上（看似"点了没反应"）。
//
// 约定 query key：
//   - item   → 日志项 ID（字符串）
//   - client → 客户端 ID（字符串）
//   - key    → 检索关键词
//   - query  → 结构化查询表达式（优先于 key）
//
// 用法：
//   const { itemID, clientID, keyword } = useRouteQuery()
//   onMounted(() => { if (itemID.value) form.item = Number(itemID.value) })
//
// 健壮性：测试环境常 mock vue-router 但只提供 useRouter，缺少 useRoute。
// 此处对 useRoute 做存在性判断，缺失时降级为空 query（等价于无跳转参数），
// 保证视图在无 query 场景下行为不变。
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'

function toPositiveInt(v) {
  const n = Number(v)
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : null
}

// resolveRoute 安全获取当前路由对象；无 useRoute 时返回 null。
//
// 某些测试 mock 只提供 useRouter，此处吞掉其抛出的 "not defined" 错误降级。
function resolveRoute() {
  try {
    return useRoute() || null
  } catch {
    return null
  }
}

// EMPTY_QUERY 无路由上下文时的安全默认（冻结防止误改）。
const EMPTY_QUERY = Object.freeze({})

export function useRouteQuery() {
  const route = resolveRoute()
  const query = route ? route.query : EMPTY_QUERY

  const itemID = ref(toPositiveInt(query.item))
  const clientID = ref(toPositiveInt(query.client))
  const keyword = ref(typeof query.key === 'string' ? query.key : '')
  const queryExpr = ref(typeof query.query === 'string' ? query.query : '')

  // 同一视图内通过 router.replace/push 改变 query 时同步更新。
  //
  // 使用 watch(getter) 而非 watch(route.query)：前者对响应式与惰性数据源均有效，
  // 且避免在 mock/非路由上下文下绑定失败。
  if (route) {
    watch(
      () => route.query,
      (q) => {
        if (!q) return
        itemID.value = toPositiveInt(q.item)
        clientID.value = toPositiveInt(q.client)
        keyword.value = typeof q.key === 'string' ? q.key : ''
        queryExpr.value = typeof q.query === 'string' ? q.query : ''
      },
      { deep: true },
    )
  }

  return { itemID, clientID, keyword, queryExpr }
}
