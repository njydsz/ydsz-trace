import { reactive } from 'vue'

const STORAGE_KEY = 'ydsz_username'

const state = reactive({
  username: localStorage.getItem(STORAGE_KEY) || '',
})

function setUser(username) {
  state.username = username || ''
  if (state.username) {
    localStorage.setItem(STORAGE_KEY, state.username)
  } else {
    localStorage.removeItem(STORAGE_KEY)
  }
}

function clear() {
  setUser('')
}

export function useAuth() {
  return {
    state,
    setUser,
    clear,
    get loggedIn() {
      return !!state.username
    },
  }
}
