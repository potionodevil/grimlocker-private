export const isDevMode = () =>
  import.meta.env.DEV ||
  new URLSearchParams(window.location.search).has('dev') ||
  localStorage.getItem('grimlocker_dev') === '1'

export const setDevMode = (enabled) => {
  if (enabled) {
    localStorage.setItem('grimlocker_dev', '1')
  } else {
    localStorage.removeItem('grimlocker_dev')
  }
}
