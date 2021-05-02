import Vue from 'vue';
import Buefy from 'buefy';
import router from './router';

import App from './views/App.vue';
import * as api from './api';

Vue.use(Buefy, {});
Vue.config.productionTip = false;

Vue.prototype.$api = api;

new Vue({
  router,
  render: (h) => h(App),

  data: {
    isLoaded: false,
    langs: {},
  },

  created() {
    this.$api.getConfig().then((res) => {
      this.langs = res.data;
      this.isLoaded = true;
    });
  },
}).$mount('#app');
