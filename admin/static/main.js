const _rootURL = '/';

const _urls = {
	api: '/api',
	admin: '/admin',
};

function formatNumber(value) {
	return value.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}


// Takes a string with linebreaks and returns an array of the lines with empty spaces
// and lines removed.
function linesToList(str) {
	if (!str) {
		return [];
	}
	return str.trim().split("\n").map((v) => v.trim()).filter(v => v !== "");
}


// Global is bound to <body> to provide a global state for all sub components.
function globalComponent() {
	return {
		// Global server config fetched from the API.
		config: {},
		urls: {},
		ready: false,

		// Map of named API calls that are set to true/false when api()
		// is called indicating loading status.
		loading: {},

		async onLoad() {
			// Fetch the server config.
			await this.api('config', `/config`).then(data => {
				this._rootURL = data.root_url;
				this.config = data;
				this.ready = true;
			});

			document.querySelector('body').style.display = 'block';
		},

		api(name, uri, method, data) {
			return new Promise((resolve, reject) => {
				this.loading[name] = true;

				fetch(`${_urls.api}${uri}`, {
					method: method || 'GET',
					body: (method === 'POST' || method == 'PUT') && data ? JSON.stringify(data) : null,
					headers: {
						"Content-Type": "application/json; charset=utf-8"
					}
				}).then(resp => {
					delete (this.loading[name]);

					// Resolve all 200 responses and return `data` from the JSON server response.
					if (resp.ok) {
						resp.json().then(data => resolve(data.data));
						return;
					}

					// For non-200 responses, return 'message' in the JSON server error response.
					resp.json().then(data => {
						alert(data.message);
						reject(Error(data.message));
						return;
					});
				}).catch((err) => {
					delete (this.loading[name]);
					reject(err);
				});
			});
		},

		makeURL(p) {
			const u = new URLSearchParams();

			if (p.guid) {
				u.set("guid", p.guid);
			}
			if (p.fromLang) {
				u.set("from_lang", p.fromLang);
			}
			if (p.toLang) {
				u.set("to_lang", p.toLang);
			}
			if (p.query) {
				u.set("query", p.query);
			}

			return `${_urls.admin}/search?${u.toString()}`;
		},

		onNewEntry() {
			this.$dispatch('open-entry-form', {
				weight: 0,
				lang: Object.keys(this.config.languages)[0],
				tokens: '',
				tags: [],
				phones: [],
				relations: [],
				status: 'enabled'
			});
		}
	}
}

function homeComponent() {
	return {
		stats: null,

		onLoad() {
			this.api('stats', '/stats').then((data) => {
				this.stats = data;
			});
		}
	}
}

// Search form component.
function searchFormComponent() {
	return {
		fromLang: localStorage.fromLang || '*guid',
		toLang: '',
		query: '',
        
		onSearch(e) {
			if (!this.query) {
				return;
			}

			// Remember from/to language seletions on the UI.
			localStorage.fromLang = this.fromLang;
			localStorage.toLang = this.toLang;

			// If GUID is selected, redirect to the GUID page. Otherwise, let the form normally submit.
			if (this.fromLang === '*guid') {
				e.preventDefault();
				document.location.href = `${_urls.admin}/search?guid=${this.query}`;
			}
		}
	}
}

function searchResultsComponent() {
	return {
		guid: null,
		query: null,
		fromLang: null,
		toLang: null,
		total: 0,
		entries: [],
		hasEntryReordered: {},

		// entryi-guid -> { changedRels{}, originalOrder[] }
		order: {},
		hasRelReordered: {},

		onLoad() {
			this.onSearch();
		},

		onSearch() {
			// Query params.
			const q = new URLSearchParams(document.location.search);
			[this.guid, this.query, this.fromLang, this.toLang] = [q.get("guid"), q.get("query"), q.get("from_lang"), q.get("to_lang")];

			// Fetch one entry buy GUID.
			if (this.guid) {
				this.api('entries.get', `/entries/${this.guid}`).then((data) => {
					this.entries = [data];
				})
			} else if (this.fromLang && this.query) {
				// Search.
				this.api('entries.search', `/dictionary/${this.fromLang}/*/${encodeURIComponent(this.query)}`).then((data) => {
					this.total = data.total;
					this.entries = data.entries;
				})
			}
		},

		onReorderRelation(entry, rel, n, d) {
			// n = index of the relation in the array.
			// d = +1 or -1 indicating direction.

			// Can't move the first item up.
			if (n === 0 && d === -1) {
				return;
			} else if (n === entry.relations.length - 1 && d === 1) {
				// Can't move the last item down.
				return;
			}

			if (!this.order.hasOwnProperty(entry.guid)) {
				this.order[entry.guid] = { original: [...entry.relations], changedRels: {} }
			}

			this.order[entry.guid].changedRels[rel.guid] = true;
			const i = entry.relations.splice(n, 1)[0];
			entry.relations.splice(n + d, 0, i);
		},

		onResetRelationOrder(entry) {
			entry.relations = [...this.order[entry.guid].original];
			delete (this.order[entry.guid]);
		},

		onSaveRelationOrder(entry) {
			const ids = entry.relations.map((r) => r.relation.id);
			this.api('entries.reorder', `/entries/${entry.guid}/relations/weights`, 'PUT', ids).then(() => {
				delete (this.order[entry.guid]);
			});
		},

		onDetatchRelation(fromGuid, toGuid) {
			if (!confirm("Detatch this definition from the above entry? It will not be deleted from the database and may still be attached to other entries.")) {
				return;
			}

			this.api('relations.detatch', `/entries/${fromGuid}/relations/${toGuid}`, 'DELETE').then(() => this.onSearch());
		},

		onEditEntry(entry, parent) {
			// this.$dispatch('open-entry-form', { ...JSON.parse(JSON.stringify(entry)), parent: parent });
			this.$dispatch('open-entry-form', { ...JSON.parse(JSON.stringify(entry)), parent: parent });
		},

		onEditRelation(rel, parent) {
			this.$dispatch('open-relation-form', { ...JSON.parse(JSON.stringify(rel)), parent: JSON.parse(JSON.stringify(parent)) });
		},

		onAddDefinition(parent) {
			this.$dispatch('open-definition-form', {
				parent: JSON.parse(JSON.stringify(parent)),

				weight: 0,
				lang: Object.keys(this.config.languages)[0],
				tokens: '',
				tags: []
			});
		},

		onDeleteEntry(guid) {
			if (!confirm("Delete this entry? The definitions are not deleted and may be attached to other entries.")) {
				return;
			}
			this.api('entries.delete', `/entries/${guid}`, 'DELETE').then(() => this.onSearch());
		},

		onDeleteRelationEntry(guid) {
			if (!confirm("Delete this entry? It will be deleted from all entries in the database it may be attached to.")) {
				return;
			}
			this.api('relations.delete', `/entries/${guid}`, 'DELETE').then(() => this.onSearch());
		}
	}
}

function entryComponent() {
	return {
		isNew: false,
		entry: null,
		parentEntries: [],
		isVisible: false,
		isFormOpen: localStorage.isFormOpen === 'true' || false,

		// This is triggered by the open-entry-form event.
		onOpen(e) {
			this.$dispatch('close-relation-form');
			this.$dispatch('close-definition-form');

			const data = e.detail;

			this.entry = {
				...data,
				phones: data.phones.join('\n'),
				tags: data.tags.join('\n'),
				tokens: data.tokens.split(' ').join('\n')
			};
			this.parentEntries = [];
			this.isNew = !this.entry.guid ? true : false;
			this.isVisible = true;

			this.$nextTick(() => {
				this.$refs.content.focus();
			});

			if (this.entry.parent) {
				this.getParentEntries(this.entry.guid);
			}
		},

		onToggleOptions() {
			this.isFormOpen = !this.isFormOpen;
			localStorage.isFormOpen = this.isFormOpen;
		},

		onFocusInitial() {
			if (!this.entry.initial && this.entry.content && this.entry.content.length > 0) {
				this.entry.initial = this.entry.content[0].toUpperCase();
			}
		},

		onClose() {
			this.isVisible = false;
		},

		onSave() {
			if (this.parentEntries.length > 1) {
				if (!confirm(`Update this definition across ${this.parentEntries.length} parent entries?`)) {
					return;
				}
			}

			let data = {
				...this.entry,
				initial: this.entry.initial ? this.entry.initial : this.entry.content[0].toUpperCase(),
				phones: linesToList(this.entry.phones),
				tags: linesToList(this.entry.tags),
				tokens: linesToList(this.entry.tokens).join(' '),
			};

			// New entry.
			if (this.isNew) {
				this.api('entries.create', `/entries`, 'POST', data).then((data) => {
					this.onClose()
					document.location.href = `${_urls.admin}/search?guid=${data.guid}`;
				});
			} else {
				this.api('entries.update', `/entries/${this.entry.guid}`, 'PUT', data).then(() => this.onClose());
			}
		},

		onDeleteEntry() {
			const msg = `Delete this definition from ${this.parentEntries.length} parent entries?`;
			if (!confirm(msg)) {
				return;
			}

			this.api('entries.delete', `/entries/${this.entry.guid}`, 'DELETE').then(() => {
				this.onClose();
			});
		},


		getParentEntries(guid) {
			this.api('entries.getParents', `/entries/${guid}/parents`).then((data) => {
				this.parentEntries = data;
			});
		}
	}
}


function relationComponent() {
	return {
		entry: null,
		isVisible: false,

		// This is triggered by the open-relation-form event.
		onOpen(e) {
			this.$dispatch('close-entry-form');
			this.$dispatch('close-definition-form-form');

			const data = e.detail;
			this.entry = {
				...data,
				relation: {
					...data.relation,
					tags: data.relation.tags.join('\n')
				},
			};
			this.isVisible = true;
		},

		onSave() {
			let data = {
				...this.entry.relation,
				types: this.entry.relation.types,
				tags: linesToList(this.entry.relation.tags),
				notes: this.entry.relation.notes
			};

			this.api('relations.update', `/entries/${this.entry.parent.guid}/relations/${this.entry.relation.id}`, 'PUT', data).then(() => {
				this.onClose();
				this.$dispatch('search');
			});
		},

		onClose() {
			this.isVisible = false;
		}
	}
}


function definitionComponent() {
	return {
		parent: null,
		def: {},
		isVisible: false,

		// This is triggered by the open-definition-form event.
		onOpen(e) {
			this.$dispatch('close-entry-form');
			this.$dispatch('close-relation-form');

			this.parent = e.detail.parent;
			delete (e.detail.parent);

			this.def = { ...e.detail, tags: e.detail.tags.join('\n') };
			this.isVisible = true;
		},

		onSave() {
			const data = {
				content: this.def.content,
				initial: this.def.content[0].toUpperCase(),
				lang: this.def.lang,
				phones: [],
				tags: [],
				status: 'enabled'
			};

			// Insert the definition entry first.
			this.api(`/entries`, 'POST', data).then((data) => {
				// Add the relation.
				const rel = {
					types: this.def.types,
					tags: linesToList(this.def.tags),
					notes: this.def.notes,
				};
				api('relations.add', `/entries/${this.parent.guid}/relations/${data.guid}`, 'POST', rel).then(() => {
					this.$dispatch('search');
					this.onClose();
				});
			});


		},

		onClose() {
			this.isVisible = false;
		}
	}
}