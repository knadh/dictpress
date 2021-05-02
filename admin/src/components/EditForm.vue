<template>
  <form @submit.prevent="onSubmit">
    <div class="modal-card content" style="width: auto">
      <header class="modal-card-head">
        <h3 class="title is-4">{{ form.content }}</h3>
        <span class="is-size-7 has-text-grey">ID: {{ data.id }} &mdash; GUID: {{ data.guid }}</span>
      </header>
      <section expanded class="modal-card-body">
        <div class="columns">
          <div class="column is-8">
            <b-field label="Entry">
              <b-input v-model="form.content" ref="content" name="content" autofocus />
            </b-field>
          </div>
          <div class="column is-4">
            <b-field label="Language">
              <b-select v-model="form.lang" name="lang" expanded>
                <option v-for="(l, id) in $root.$data.langs" :key="id" :value="id">
                  {{ l.name }}
                </option>
              </b-select>
            </b-field>
          </div>
        </div><!-- columns -->

        <div>
            <b-field label="Phones">
              <b-taginput v-model="form.phones" clearable />
            </b-field>

            <b-field label="Tags">
              <b-taginput v-model="form.tags" clearable />
            </b-field>

            <b-field label="Types">
              <b-taginput
                v-model="form.types"
                :data="types[form.lang]"
                field="name"
                autocomplete
                open-on-focus
                clearable
              />
            </b-field>

            <b-field label="Notes">
              <b-input v-model="form.notes" type="textarea" name="notes" />
            </b-field>
        </div>
      </section>
      <footer class="modal-card-foot has-text-right">
        <b-button @click="$parent.close()">Cancel</b-button>
        <b-button native-type="submit" type="is-primary">Save</b-button>
      </footer>
    </div>
  </form>
</template>

<script>
import Vue from 'vue';

const TAG_TYPE = {
  enabled: 'is-success',
  pending: 'is-warning',
  disabled: 'is-error',
};

export default Vue.extend({
  name: 'EditForm',

  props: {
    data: {},
    isEditing: null,
  },

  data() {
    return {
      form: {
        content: '',
        lang: '',
        phones: [],
        types: [],
        tags: [],
        notes: '',
      },

      // english: [{ id: noun, name: Noun} ...]
      types: {},
    };
  },

  created() {
    // Prepare the list of types for all languages for the taginput.
    Object.entries(this.$root.$data.langs).forEach((l) => {
      const types = [];
      Object.entries(l[1].types).forEach((t) => {
        types.push({ id: t[0], name: t[1] });
      });
      this.types[l[0]] = types;
    });

    this.form = {
      ...this.form,
      content: this.$props.data.content,
      lang: this.$props.data.lang,
      phones: this.$props.data.phones,
      types: this.entryTypes(this.$props.data.lang, this.$props.data.types),
      tags: this.$props.data.tags,
      notes: this.$props.data.notes,
    };
  },

  mounted() {
    this.$nextTick(() => {
      this.$refs.content.focus();
    });
  },

  methods: {
    onSubmit() {
      console.log(this.form);
    },

    entryTypes(lang, types) {
      return this.types[lang].filter((t) => types.indexOf(t.id) !== -1);
    },

    tagType(typ) {
      return TAG_TYPE[typ];
    },
  },

  watch: {
    'form.lang': function () {
      if (this.form.lang !== this.data.lang) {
        console.log(this.form.lang);
        this.form.types = this.entryTypes(this.form.lang, this.form.types);
      }
    },
  },
});
</script>
