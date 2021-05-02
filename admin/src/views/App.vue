<template>
  <div id="app">
    <b-navbar :fixed-top="true">
      <template slot="brand">
        <div class="logo">
          <router-link :to="{name: 'dashboard'}">
            <img class="favicon" src="@/assets/favicon.png"/>
          </router-link>
        </div>
      </template>
      <template slot="start">
        <b-navbar-item tag="div">
            <router-link :to="{name: 'dashboard'}">New</router-link>
        </b-navbar-item>
        <b-navbar-item tag="div">
            <router-link :to="{name: 'dashboard'}">Edits</router-link>
        </b-navbar-item>
      </template>
      <template slot="end">
        <b-navbar-item tag="form" @submit.prevent="onSearch">
          <b-select v-model="searchLang" name="lang">
            <option value="=">*GUID</option>
            <option v-for="(l, id) in $root.$data.langs" :key="id" :value="id">
              {{ l.name }}
            </option>
          </b-select>
          <b-input v-model="searchQuery" name="query" placeholder="Search" />
        </b-navbar-item>
      </template>
    </b-navbar>

    <router-view :key="$route.fullPath" v-if="$root.$data.isLoaded" />
  </div>
</template>

<script>
import Vue from 'vue';

export default Vue.extend({
  name: 'App',

  data() {
    return {
      searchLang: '=',
      searchQuery: '',
      langs: [],
    };
  },

  mounted() {
    // this.$route.query is empty randomly for whatever reason, so
    // use a timer to access it.
    this.$nextTick(() => {
      window.setTimeout(() => {
        if (this.$route.query.lang) {
          this.searchLang = this.$route.query.lang;
        }
      }, 100);
    });
  },

  methods: {
    onSearch() {
      this.$router.push({
        name: 'search',
        query: {
          lang: this.searchLang,
          q: this.searchQuery,
        },
      });
    },
  },
});
</script>

<style lang="scss">
  @import '../assets/style.scss';
</style>
