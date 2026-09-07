const GalleryView = {
    template: `
    <div>
        <div class="top-bar">
            <h1>Gallery</h1>
            <label class="junk-toggle">
                <input type="checkbox" v-model="hideJunk" @change="resetAndLoad" />
                <span>Hide junk</span>
            </label>
            <label class="junk-toggle">
                <input type="checkbox" v-model="hasFace" @change="resetAndLoad" />
                <span>Has face</span>
            </label>
            <div class="chip-row">
                <div class="chip" :class="{active: !category}" @click="setCategory('')">All</div>
                <div v-for="c in categories" :key="c"
                     class="chip" :class="{active: category === c}"
                     @click="setCategory(c)">
                    {{ shortName(c) }}
                </div>
            </div>
            <div class="chip-row" v-if="yearMonths.length">
                <div class="chip" :class="{active: !yearMonth}" @click="setYearMonth('')">All months</div>
                <div v-for="ym in yearMonths" :key="ym"
                     class="chip" :class="{active: yearMonth === ym}"
                     @click="setYearMonth(ym)">
                    {{ formatYM(ym) }}
                </div>
            </div>
        </div>
        <thumb-grid
            :files="files"
            :loading="loading"
            :has-more="hasMore"
            @load-more="loadMore"
            @open-file="openFile"
        ></thumb-grid>
    </div>
    `,
    setup() {
        const files = Vue.ref([]);
        const loading = Vue.ref(false);
        const cursor = Vue.ref('');
        const hasMore = Vue.ref(false);
        const category = Vue.ref('');
        const yearMonth = Vue.ref('');
        const categories = Vue.ref([]);
        const yearMonths = Vue.ref([]);
        const hideJunk = Vue.ref(true);
        const hasFace = Vue.ref(false);

        async function loadCategories() {
            try {
                const data = await fetchJSON('/api/categories');
                categories.value = data.categories || [];
            } catch (e) {}
        }

        async function loadYearMonths() {
            try {
                const url = '/api/yearmonths' + (category.value ? '?category=' + encodeURIComponent(category.value) : '');
                const data = await fetchJSON(url);
                yearMonths.value = data.year_months || [];
            } catch (e) {}
        }

        async function load() {
            loading.value = true;
            try {
                let url = '/api/browse?limit=50';
                if (category.value) url += '&category=' + encodeURIComponent(category.value);
                if (yearMonth.value) url += '&ym=' + encodeURIComponent(yearMonth.value);
                if (cursor.value) url += '&cursor=' + encodeURIComponent(cursor.value);
                if (hideJunk.value) url += '&hide_junk=1';
                if (hasFace.value) url += '&has_face=1';
                const data = await fetchJSON(url);
                files.value = files.value.concat(data.files || []);
                cursor.value = data.next_cursor || '';
                hasMore.value = !!data.next_cursor;
                // Keep lightbox files in sync if lightbox is open
                if (galleryState.lightboxFile) {
                    galleryState.lightboxFiles = files.value;
                }
            } catch (e) { console.error(e); }
            loading.value = false;
        }

        function loadMore() {
            if (hasMore.value && !loading.value) load();
        }

        function resetAndLoad() {
            files.value = [];
            cursor.value = '';
            hasMore.value = false;
            load();
        }

        function setCategory(c) {
            category.value = c;
            yearMonth.value = '';
            resetAndLoad();
            loadYearMonths();
        }

        function setYearMonth(ym) {
            yearMonth.value = ym;
            resetAndLoad();
        }

        function openFile(file) {
            galleryState.lightboxFiles = files.value;
            galleryState.lightboxIndex = files.value.findIndex(f => f.sha256 === file.sha256 && f.relpath === file.relpath);
            galleryState.lightboxFile = file;
        }

        Vue.onMounted(() => {
            loadCategories();
            loadYearMonths();
            load();
        });

        // Auto-load more when lightbox navigation approaches the end
        Vue.watch(() => galleryState.lightboxIndex, (idx) => {
            if (idx < 0 || !galleryState.lightboxFile) return;
            const total = galleryState.lightboxFiles.length;
            if (hasMore.value && !loading.value && idx >= total - 10) {
                load();
            }
        });

        // Also sync lightboxFiles when new files arrive while lightbox is open
        Vue.watch(files, () => {
            if (galleryState.lightboxFile) {
                galleryState.lightboxFiles = files.value;
            }
        }, { deep: false });

        return { files, loading, hasMore, category, yearMonth, categories, yearMonths, hideJunk, hasFace,
                 loadMore, setCategory, setYearMonth, resetAndLoad, openFile, shortName, formatYM };
    }
};
