const SearchView = {
    template: `
    <div>
        <div class="top-bar">
            <h1>Search</h1>
            <div class="mt-3">
                <input v-model="query" @keyup.enter="search"
                    placeholder="Search file paths..."
                    class="search-input"
                    ref="searchInput">
            </div>
        </div>
        <div class="p-4">
            <p v-if="results" class="text-sm text-neutral-500 mb-4">
                {{ results.count }} results for "{{ results.query }}"
            </p>
            <thumb-grid
                v-if="results"
                :files="results.files"
                :loading="loading"
                :has-more="false"
                @open-file="openFile"
            ></thumb-grid>
            <div v-if="!results && !loading" class="text-center text-neutral-600 py-20">
                <i data-lucide="search" class="w-12 h-12 mx-auto mb-4 opacity-30"></i>
                <p class="text-sm">Start typing to search</p>
                <div class="mt-4 flex flex-wrap gap-2 justify-center">
                    <span class="chip" @click="quickSearch('2024')">2024</span>
                    <span class="chip" @click="quickSearch('DJI_')">DJI_</span>
                    <span class="chip" @click="quickSearch('IMG_')">IMG_</span>
                    <span class="chip" @click="quickSearch('MVI_')">Videos</span>
                </div>
            </div>
        </div>
    </div>
    `,
    setup() {
        const query = Vue.ref('');
        const results = Vue.ref(null);
        const loading = Vue.ref(false);
        const searchInput = Vue.ref(null);

        async function search() {
            if (!query.value.trim()) return;
            loading.value = true;
            try {
                results.value = await fetchJSON('/api/search?q=' + encodeURIComponent(query.value) + '&limit=100');
            } catch (e) { console.error(e); }
            loading.value = false;
        }

        function quickSearch(q) {
            query.value = q;
            search();
        }

        function openFile(file) { galleryState.lightboxFile = file; }

        Vue.onMounted(() => {
            lucide.createIcons();
            // auto-focus search on mobile
            Vue.nextTick(() => {
                if (searchInput.value) searchInput.value.focus();
            });
        });

        return { query, results, loading, search, quickSearch, openFile, searchInput };
    }
};
