import DefaultTheme from 'vitepress/theme'
import Shot from './Shot.vue'
import Walkthrough from './Walkthrough.vue'
import RequestFlow from './RequestFlow.vue'
import SetupOverview from './SetupOverview.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('Shot', Shot)
    app.component('Walkthrough', Walkthrough)
    app.component('RequestFlow', RequestFlow)
    app.component('SetupOverview', SetupOverview)
  },
}
