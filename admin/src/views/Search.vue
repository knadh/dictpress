<template>
  <div class="container">
    <header class="page-header">
      <h2 class="title is-4">Search &ldquo;{{ searchQuery }}&rdquo;</h2>
      <h3>{{ data.total }} result(s)</h3>
    </header>
    <section class="page-body">
      <ol class="entries no">
        <li v-for="e in data.entries" :key="e.guid" class="box">
          <div class="columns">
            <div class="column is-6">
              <h3 class="title is-5">
                {{ e.content }}
                <b-tag :type="tagType(e.status)">{{ e.status }}</b-tag>
              </h3>
              <p class="phone has-text-grey">♪ {{ e.phones.join(", ") }}</p>
            </div>
            <div class="column is-5 has-text-right">
              <span class="is-size-7 has-text-grey">ID: {{ e.id }} &mdash; GUID: {{ e.guid }}</span>
            </div>
            <div class="column is-1 has-text-right">
              <a href="" @click.prevent="showEditForm(e)">
                <b-tooltip label="Edit" type="is-dark"><span class="icon">✎</span></b-tooltip>
              </a>
              <a href="">
                <b-tooltip label="Delete" type="is-dark"><span class="icon">×</span></b-tooltip>
              </a>
            </div>
          </div><!-- columns -->

          <ol class="defs">
            <li v-for="d in e.relations" :key="d.guid">
              <div class="columns">
                <div class="column is-8">
                  <p>{{ d.content }}</p>
                  <p>
                    <b-taglist>
                      <b-tag class="is-light" v-for="t in d.types" :key="t">
                        {{ $root.$data.langs[d.lang].types[t] }}
                      </b-tag>
                    </b-taglist>
                  </p>
                </div>
                <div class="column is-4 has-text-right">
                  <a href="" @click.prevent="showEditForm(d)">
                    <b-tooltip label="Edit" type="is-dark"><span class="icon">✎</span></b-tooltip>
                  </a>
                  <a href="">
                    <b-tooltip label="Delete" type="is-dark"><span class="icon">×</span></b-tooltip>
                  </a>
                </div>
              </div><!-- columns -->
            </li>
          </ol>
        </li>
      </ol>
    </section>

    <!-- Add / edit form modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isFormVisible" :width="600">
      <edit-form :data="curEntry" :isEditing="isEditing" @finished="formFinished"></edit-form>
    </b-modal>
  </div>
</template>

<script>
import Vue from 'vue';
import EditForm from '../components/EditForm.vue';

const TAG_TYPE = {
  enabled: 'is-success',
  pending: 'is-warning',
  disabled: 'is-error',
};

export default Vue.extend({
  name: 'Search',

  components: {
    EditForm,
  },

  data() {
    return {
      searchLang: '',
      searchQuery: '',
      data: {},

      curEntry: {},
      isEditing: false,
      isFormVisible: false,

      // english: [{ id: noun, name: Noun} ...]
      types: {},
    };
  },

  created() {
    if (this.$route.query.lang && this.$route.query.q) {
      this.searchLang = this.$route.query.lang;
      this.searchQuery = this.$route.query.q;
      this.$api.search(this.$route.query.q, this.$route.query.lang, '*').then((res) => {
        this.data = res.data;
      });
    }

    // Prepare the list of types for all languages for the taginput.
    Object.entries(this.$root.$data.langs).forEach((l) => {
      const types = [];
      Object.entries(l[1].types).forEach((t) => {
        types.push({ id: t[0], name: t[1] });
      });
      this.types[l[0]] = types;
    });
  },

  methods: {
    entryTypes(lang, types) {
      return this.types[lang].filter((t) => types.indexOf(t.id) !== -1);
    },

    tagType(typ) {
      return TAG_TYPE[typ];
    },

    formFinished() {
      this.isFormVisible = false;
      this.curEntry = null;
      this.isEditing = false;
    },

    showEditForm(e) {
      this.curEntry = e;
      this.isEditing = true;
      this.isFormVisible = true;
    },
  },
});
</script>
