import Vue from 'vue';
import VueRouter from 'vue-router';

Vue.use(VueRouter);

// The meta.group param is used in App.vue to expand menu group by name.
const routes = [
  {
    path: '/',
    name: 'home',
    meta: { title: 'home' },
    component: () => import(/* webpackChunkName: "main" */ './views/Dashboard.vue'),
  },
  {
    path: '/search',
    name: 'search',
    meta: { title: 'Search' },
    component: () => import(/* webpackChunkName: "main" */ './views/Search.vue'),
  },
];

const router = new VueRouter({
  mode: 'history',
  base: process.env.BASE_URL,
  routes,

  scrollBehavior(to) {
    if (to.hash) {
      return { selector: to.hash };
    }
    return { x: 0, y: 0 };
  },
});

router.afterEach((to) => {
  Vue.nextTick(() => {
    document.title = `${to.meta.title}`;
  });
});

export default router;
