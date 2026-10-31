import DefaultTheme from 'vitepress/theme'
import Shot from './Shot.vue'
import Walkthrough from './Walkthrough.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('Shot', Shot)
    app.component('Walkthrough', Walkthrough)
  },
}
