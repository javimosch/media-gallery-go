const DupesView = {
    template: `
    <div>
        <div class="top-bar">
            <h1>Duplicates</h1>
            <p class="text-xs text-neutral-500 mt-1" v-if="stats.dupe_groups">
                {{ stats.dupe_groups }} groups · {{ formatSize(stats.dupe_size) }} wasted
            </p>
        </div>
        <div class="p-4">
            <div v-if="loading && groups.length === 0" class="loading-spinner"></div>
            <div class="space-y-3">
                <div v-for="group in groups" :key="group.sha256"
                     class="dupe-card">
                    <div class="flex items-center justify-between mb-3">
                        <div class="text-sm text-neutral-400">
                            {{ group.count }} copies · {{ formatSize(group.size) }}
                        </div>
                        <button @click="toggleGroup(group.sha256)"
                            class="chip" :class="{active: expanded === group.sha256}">
                            {{ expanded === group.sha256 ? 'Hide' : 'Show' }}
                        </button>
                    </div>
                    <div v-if="expanded === group.sha256" class="thumb-grid mt-3">
                        <div v-for="file in groupFiles" :key="file.relpath"
                             class="thumb-item" @click="openFile(file)">
                            <img :src="'/api/thumb/' + file.sha256" loading="lazy" draggable="false" @error="onErr">
                        </div>
                    </div>
                </div>
            </div>
            <button v-if="groups.length > 0 && !loading" class="load-more-btn" @click="loadMore">
                Load more groups
            </button>
        </div>
    </div>
    `,
    setup() {
        const groups = Vue.ref([]);
        const loading = Vue.ref(false);
        const offset = Vue.ref(0);
        const expanded = Vue.ref('');
        const groupFiles = Vue.ref([]);
        const stats = Vue.ref({});

        async function loadStats() {
            try { stats.value = await fetchJSON('/api/stats'); } catch (e) {}
        }

        async function load() {
            loading.value = true;
            try {
                const data = await fetchJSON('/api/dupes?limit=20&offset=' + offset.value);
                groups.value = groups.value.concat(data.groups || []);
            } catch (e) { console.error(e); }
            loading.value = false;
        }

        async function toggleGroup(sha) {
            if (expanded.value === sha) { expanded.value = ''; groupFiles.value = []; return; }
            expanded.value = sha;
            try {
                const data = await fetchJSON('/api/dupes/' + sha);
                groupFiles.value = data.files || [];
            } catch (e) { groupFiles.value = []; }
        }

        function loadMore() { offset.value += 20; load(); }
        function openFile(file) { galleryState.lightboxFile = file; }
        function onErr(e) { e.target.style.opacity = '0.2'; }
        function formatSize(b) {
            if (!b) return '0 B';
            if (b < 1048576) return (b / 1024).toFixed(0) + ' KB';
            if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
            return (b / 1073741824).toFixed(2) + ' GB';
        }

        Vue.onMounted(() => { loadStats(); load(); lucide.createIcons(); });
        Vue.onUpdated(() => lucide.createIcons());

        return { groups, loading, expanded, groupFiles, stats, toggleGroup, loadMore, openFile, onErr, formatSize };
    }
};
