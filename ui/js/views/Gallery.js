const GalleryView = {
    template: `
    <div>
        <div class="top-bar">
            <h1>Gallery</h1>
            <label class="junk-toggle">
                <input type="checkbox" v-model="hideJunk" @change="toggleJunk" />
                <span>Hide junk</span>
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
                const data = await fetchJSON(url);
                files.value = files.value.concat(data.files || []);
                cursor.value = data.next_cursor || '';
                hasMore.value = !!data.next_cursor;
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

        function toggleJunk() {
            resetAndLoad();
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

        function openFile(file) { galleryState.lightboxFile = file; }

        Vue.onMounted(() => {
            loadCategories();
            loadYearMonths();
            load();
        });

        return { files, loading, hasMore, category, yearMonth, categories, yearMonths, hideJunk,
                 loadMore, setCategory, setYearMonth, toggleJunk, openFile, shortName, formatYM };
    }
};
