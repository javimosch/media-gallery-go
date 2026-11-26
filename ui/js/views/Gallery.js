const GalleryView = {
    template: `
    <div>
        <div class="top-bar gallery-head" :style="accentStyle">
            <div class="head-title">
                <h1>Gallery</h1>
                <button v-if="signedInAs" class="sign-out" @click="signOut">{{ signedInAs }} · Sign out</button>
            </div>
            <div class="group-bar" v-if="groups.length" role="group" aria-label="Category">
                <button class="group-btn" :aria-pressed="!group" @click="setGroup('')">
                    <b>All</b><small v-if="allCount">{{ allCount.toLocaleString() }}</small>
                </button>
                <button v-for="g in groups" :key="g.name" class="group-btn" :aria-pressed="group === g.name" @click="setGroup(g.name)">
                    <b>{{ g.name }}</b><small>{{ g.count.toLocaleString() }}</small>
                </button>
            </div>
            <input v-model="nameQuery" type="search" class="search-input" aria-label="Search by filename"
                   placeholder="Search by filename, e.g. IMG_20260101" @input="onNameInput">
            <p v-if="nameQuery.trim() && !loading && !failed" class="head-note">
                Searching {{ group || 'the whole gallery' }}, junk included.
                <span v-if="category || yearMonth || device || !showPhotos || !showVideos || hasFace">Other filters are on, clear them to search all of it.</span>
            </p>
            <div class="head-row">
                <span class="head-label">Folder</span>
                <div class="chip-row">
                    <div class="chip" :class="{active: !category}" @click="setCategory('')">{{ group ? 'All ' + group : 'All folders' }}</div>
                    <div v-for="f in folders" :key="f.name" :title="f.name"
                         class="chip" :class="{active: category === f.name}"
                         @click="setCategory(f.name)">
                        {{ folderLabel(f.name) }}<small v-if="f.count">{{ f.count.toLocaleString() }}</small>
                    </div>
                </div>
            </div>
            <div class="head-row" v-if="yearMonths.length">
                <span class="head-label">Month</span>
                <div class="chip-row">
                    <div class="chip" :class="{active: !yearMonth}" @click="setYearMonth('')">All months</div>
                    <div v-for="ym in yearMonths" :key="ym"
                         class="chip" :class="{active: yearMonth === ym}"
                         @click="setYearMonth(ym)">
                        {{ formatYM(ym) }}
                    </div>
                </div>
            </div>
            <div class="head-row" v-if="devices.length">
                <label class="head-label" for="device-select">Device</label>
                <select id="device-select" class="device-select" v-model="device" @change="changeDevice">
                    <option value="">Any device</option>
                    <optgroup v-for="g in deviceGroups" :key="g.title" :label="g.title">
                        <option v-for="d in g.items" :key="d.value" :value="d.value">{{ d.label }} ({{ d.count.toLocaleString('en-GB') }})</option>
                    </optgroup>
                </select>
            </div>
            <div class="head-toggles">
                <label class="junk-toggle">
                    <input type="checkbox" v-model="showPhotos" @change="resetAndLoad" />
                    <span>Show photos</span>
                </label>
                <label class="junk-toggle">
                    <input type="checkbox" v-model="showVideos" @change="resetAndLoad" />
                    <span>Show videos</span>
                </label>
                <label class="junk-toggle">
                    <input type="checkbox" v-model="showJunk" @change="resetAndLoad" />
                    <span>Show junk</span>
                </label>
                <label class="junk-toggle">
                    <input type="checkbox" v-model="hasFace" @change="resetAndLoad" />
                    <span>Has face</span>
                </label>
            </div>
        </div>
        <p class="result-bar" aria-live="polite">
            <template v-if="failed">Could not load results, try again</template>
            <template v-else-if="!filteredAt">Loading…</template>
            <template v-else><b>{{ total.toLocaleString('en-GB') }}</b> {{ total === 1 ? 'match' : 'matches' }} · filtered {{ filteredAt }} (Paris time)</template>
        </p>
        <div class="month-nav" v-if="yearMonth && newerMonth && !failed">
            <button class="load-more-btn month-btn" @click="goMonth(newerMonth)">↑ Next month: {{ formatYM(newerMonth) }}</button>
        </div>
        <thumb-grid
            dated
            :segmented="!yearMonth"
            :files="files"
            :loading="loading"
            :has-more="hasMore"
            :show-names="!!nameQuery.trim()"
            @load-more="loadMore"
            @open-file="openFile"
        ></thumb-grid>
        <div class="month-nav" v-if="yearMonth && olderMonth && filteredAt && !loading && !hasMore && !failed">
            <button class="load-more-btn month-btn" @click="goMonth(olderMonth)">↓ Previous month: {{ formatYM(olderMonth) }}</button>
        </div>
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
        const showPhotos = Vue.ref(true);
        const showVideos = Vue.ref(true);
        const showJunk = Vue.ref(false);
        const hasFace = Vue.ref(false);

        const devices = Vue.ref([]);
        const device = Vue.ref('');
        const DEVICE_TYPES = [['phone', 'Phones'], ['camera', 'Cameras'], ['drone', 'Drones'],
            ['action camera', 'Action cameras'], ['scanner', 'Scanners and software'], ['other', 'Other'], ['unknown', 'Unknown']];
        const deviceGroups = Vue.computed(() => {
            const known = DEVICE_TYPES.map(t => t[0]);
            const typeOf = d => known.includes(d.type) ? d.type : 'other';
            return DEVICE_TYPES.map(([type, title]) => ({ title, items: devices.value.filter(d => typeOf(d) === type) }))
                .filter(g => g.items.length);
        });
        let deviceRequest = 0;

        // The list follows group, folder and month. A selected device that is not
        // in the new view is cleared, and the results reload without it.
        async function loadDevices() {
            const request = ++deviceRequest;
            const params = new URLSearchParams();
            if (group.value) params.set('group', group.value);
            if (category.value) params.set('category', category.value);
            if (yearMonth.value) params.set('ym', yearMonth.value);
            try {
                const data = await fetchJSON('/api/devices?' + params);
                if (request !== deviceRequest) return;
                devices.value = data.devices || [];
                if (device.value && !devices.value.some(d => d.value === device.value)) {
                    device.value = '';
                    resetAndLoad();
                    loadYearMonths();
                }
            } catch (e) { console.error(e); }
        }

        // months are listed newest first; "next" is newer, "previous" is older
        const newerMonth = Vue.computed(() => yearMonths.value.filter(m => m > yearMonth.value).pop() || '');
        const olderMonth = Vue.computed(() => yearMonths.value.find(m => m < yearMonth.value) || '');

        function goMonth(ym) {
            const scroller = document.querySelector('.scroll-container');
            if (scroller) scroller.scrollTop = 0;
            window.scrollTo(0, 0);
            setYearMonth(ym);
        }

        function changeDevice() {
            resetAndLoad();
            loadYearMonths();
        }

        // only when the server requires sign-in; otherwise /auth/me does not exist
        const signedInAs = Vue.ref('');
        fetch('/auth/me', { cache: 'no-store' })
            .then(res => res.ok ? res.json() : null)
            .then(me => { signedInAs.value = (me && me.user) || ''; })
            .catch(() => {});

        async function signOut() {
            try {
                const res = await fetch('/auth/logout', { method: 'POST' });
                if (res.ok || res.status === 401) location.replace('/login');
            } catch (e) { console.error(e); }
        }

        const GROUP_KEY = 'mg.group';
        const groups = Vue.ref([]);
        const group = Vue.ref('');
        const allCount = Vue.ref(0);

        // storage can throw (private window, blocked site data)
        function storedGroup() {
            try { return localStorage.getItem(GROUP_KEY) || ''; } catch (e) { return ''; }
        }
        function storeGroup(name) {
            try { localStorage.setItem(GROUP_KEY, name); } catch (e) {}
        }

        const folders = Vue.computed(() => {
            if (!groups.value.length) return categories.value.map(name => ({ name }));
            if (group.value) return (groups.value.find(g => g.name === group.value) || {}).folders || [];
            return categories.value.map(name => {
                for (const g of groups.value) {
                    const f = g.folders.find(x => x.name === name);
                    if (f) return f;
                }
                return { name };
            });
        });

        // inside a group, drop a folder prefix that only repeats the group name
        function folderLabel(name) {
            const prefix = group.value.toLowerCase() + '-';
            if (group.value && name.toLowerCase().startsWith(prefix)) name = name.slice(prefix.length);
            return shortName(name);
        }

        const ACCENTS = ['#f2a36b', '#5fd0b5', '#c3a6ff', '#e8d26a'];
        const accentStyle = Vue.computed(() => {
            const i = groups.value.findIndex(g => g.name === group.value);
            return { '--accent': i < 0 ? '#7aa2ff' : ACCENTS[i % ACCENTS.length] };
        });

        function setGroup(name) {
            group.value = name;
            storeGroup(name);
            category.value = '';
            yearMonth.value = '';
            resetAndLoad();
            loadYearMonths();
            loadDevices();
        }

        async function loadCategories() {
            try {
                const data = await fetchJSON('/api/categories');
                categories.value = data.categories || [];
                groups.value = data.groups || [];
                allCount.value = data.total || 0;
            } catch (e) {}
        }

        async function loadYearMonths() {
            try {
                const wanted = category.value + '|' + group.value + '|' + device.value;
                const params = new URLSearchParams();
                if (category.value) params.set('category', category.value);
                if (group.value) params.set('group', group.value);
                if (device.value) params.set('device', device.value);
                const data = await fetchJSON('/api/yearmonths?' + params);
                if (wanted !== category.value + '|' + group.value + '|' + device.value) return;
                yearMonths.value = data.year_months || [];
            } catch (e) {}
        }

        const nameQuery = Vue.ref('');
        const total = Vue.ref(0);
        const failed = Vue.ref(false);
        // when the current filter's results arrived; empty while they load
        const filteredAt = Vue.ref('');
        const parisTime = new Intl.DateTimeFormat('en-GB', {
            timeZone: 'Europe/Paris', day: '2-digit', month: 'short', year: 'numeric',
            hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
        });
        let typingTimer = null;

        function onNameInput() {
            filteredAt.value = '';
            // invalidate in-flight results at once so no stale count is shown
            generation++;
            loading.value = true;
            clearTimeout(typingTimer);
            typingTimer = setTimeout(resetAndLoad, 300);
        }

        // Drops responses from requests started before the last filter change
        let generation = 0;

        async function load() {
            const gen = generation;
            loading.value = true;
            failed.value = false;
            try {
                let url = '/api/browse?limit=50';
                if (group.value) url += '&group=' + encodeURIComponent(group.value);
                if (category.value) url += '&category=' + encodeURIComponent(category.value);
                if (yearMonth.value) url += '&ym=' + encodeURIComponent(yearMonth.value);
                if (cursor.value) url += '&cursor=' + encodeURIComponent(cursor.value);
                if (nameQuery.value.trim()) url += '&name=' + encodeURIComponent(nameQuery.value.trim());
                if (!showPhotos.value) url += '&photos=0';
                if (!showVideos.value) url += '&videos=0';
                url += showJunk.value ? '&junk=include' : '&hide_junk=1';
                if (hasFace.value) url += '&has_face=1';
                if (device.value) url += '&device=' + encodeURIComponent(device.value);
                const data = await fetchJSON(url);
                if (gen !== generation) return;
                total.value = data.total || 0;
                if (!filteredAt.value) filteredAt.value = parisTime.format(new Date());
                files.value = files.value.concat(data.files || []);
                cursor.value = data.next_cursor || '';
                hasMore.value = !!data.next_cursor;
                // Keep lightbox files in sync if lightbox is open
                if (galleryState.lightboxFile) {
                    galleryState.lightboxFiles = files.value;
                }
            } catch (e) {
                console.error(e);
                if (gen !== generation) return;
                failed.value = true;
            }
            loading.value = false;
        }

        function loadMore() {
            if (hasMore.value && !loading.value) load();
        }

        function resetAndLoad() {
            generation++;
            filteredAt.value = '';
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
            loadDevices();
        }

        function setYearMonth(ym) {
            yearMonth.value = ym;
            resetAndLoad();
            loadDevices();
        }

        function openFile(file) {
            galleryState.lightboxFiles = files.value;
            galleryState.lightboxIndex = files.value.findIndex(f => f.sha256 === file.sha256 && f.relpath === file.relpath);
            galleryState.lightboxFile = file;
        }

        Vue.onMounted(async () => {
            // restore the group first so the first listing is already filtered
            loading.value = true;
            await loadCategories();
            const saved = storedGroup();
            if (groups.value.some(g => g.name === saved)) group.value = saved;
            loadYearMonths();
            loadDevices();
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

        return { signedInAs, signOut, newerMonth, olderMonth, goMonth, changeDevice, devices, device, deviceGroups, filteredAt, groups, group, allCount, folders, folderLabel, accentStyle, setGroup, files, loading, hasMore, category, yearMonth, categories, yearMonths, showPhotos, showVideos, showJunk, nameQuery, total, failed, onNameInput, hasFace,
                 loadMore, setCategory, setYearMonth, resetAndLoad, openFile, formatYM };
    }
};
