const FacesView = {
    template: `
    <div>
        <div class="top-bar">
            <h1>Faces</h1>
            <p class="text-xs text-neutral-500 mt-1" v-if="status">
                {{ status.faces_found?.toLocaleString() }} faces in {{ status.scanned?.toLocaleString() }} photos
                · {{ status.progress_pct }}% scanned
            </p>
        </div>
        <div class="p-4">
            <!-- Loading -->
            <div v-if="loading && faces.length === 0" class="loading-spinner"></div>

            <!-- No faces db -->
            <div v-if="error === 'disabled'" class="text-center text-neutral-600 py-20">
                <i data-lucide="scan-face" class="w-12 h-12 mx-auto mb-4 opacity-40"></i>
                <p class="text-sm">Face recognition not enabled</p>
                <p class="text-xs mt-2 text-neutral-700">Start the server with -faces-db flag</p>
            </div>

            <!-- Face grid -->
            <div v-if="faces.length > 0 && !selectedFace" class="face-grid">
                <div v-for="face in faces" :key="face.id"
                     class="face-item"
                     @click="selectFace(face)">
                    <img :src="'/api/face-thumb/' + face.id" loading="lazy" draggable="false" @error="onErr">
                    <div class="face-badge" v-if="face.gender >= 0">
                        {{ face.gender === 0 ? 'F' : 'M' }}{{ face.age > 0 ? ' ' + face.age : '' }}
                    </div>
                </div>
            </div>

            <!-- Selected face: show similar -->
            <div v-if="selectedFace">
                <div class="flex items-center gap-3 mb-4">
                    <button @click="selectedFace = null; similar = []" class="chip">
                        <i data-lucide="arrow-left" class="w-3 h-3 inline mr-1"></i>Back
                    </button>
                    <span class="text-sm text-neutral-400">
                        {{ similar.length }} similar faces ({{ threshold }}% match)
                    </span>
                </div>
                <!-- Threshold slider -->
                <div class="mb-4 flex items-center gap-3">
                    <span class="text-xs text-neutral-500">Threshold</span>
                    <input type="range" min="40" max="90" v-model.number="thresholdPct"
                           @change="searchSimilar" class="flex-1 max-w-xs">
                    <span class="text-xs text-neutral-400">{{ thresholdPct }}%</span>
                </div>
                <div class="face-grid">
                    <div v-for="face in similar" :key="face.id"
                         class="face-item"
                         @click="openFile(face)">
                        <img :src="'/api/face-thumb/' + face.id" loading="lazy" draggable="false" @error="onErr">
                        <div class="face-badge">{{ face.similarity }}%</div>
                    </div>
                </div>
            </div>

            <!-- Load more -->
            <button v-if="faces.length > 0 && !selectedFace && hasMore" class="load-more-btn" @click="loadMore">
                Load more faces
            </button>
        </div>
    </div>
    `,
    setup() {
        const faces = Vue.ref([]);
        const loading = Vue.ref(false);
        const error = Vue.ref('');
        const status = Vue.ref(null);
        const offset = Vue.ref(0);
        const hasMore = Vue.ref(true);
        const selectedFace = Vue.ref(null);
        const similar = Vue.ref([]);
        const thresholdPct = Vue.ref(55);

        const threshold = Vue.computed(() => thresholdPct.value / 100);

        async function loadStatus() {
            try { status.value = await fetchJSON('/api/faces/status'); }
            catch (e) {}
        }

        async function load() {
            loading.value = true;
            error.value = '';
            try {
                const data = await fetchJSON('/api/faces?limit=60&offset=' + offset.value);
                faces.value = faces.value.concat(data.faces || []);
                hasMore.value = (data.faces || []).length === 60;
            } catch (e) {
                if (e.message.includes('503') || e.message.includes('not enabled')) error.value = 'disabled';
            }
            loading.value = false;
        }

        function loadMore() { offset.value += 60; load(); }

        async function selectFace(face) {
            selectedFace.value = face;
            similar.value = [];
            await searchSimilar();
        }

        async function searchSimilar() {
            if (!selectedFace.value) return;
            try {
                const data = await fetchJSON(`/api/faces/${selectedFace.value.id}/similar?threshold=${threshold.value}&limit=200`);
                similar.value = data.faces || [];
            } catch (e) { console.error(e); }
        }

        function openFile(face) {
            galleryState.lightboxFile = { sha256: face.sha256, relpath: face.relpath, size: 0 };
        }

        function onErr(e) { e.target.style.opacity = '0.2'; }

        Vue.onMounted(() => { loadStatus(); load(); lucide.createIcons(); });
        Vue.onUpdated(() => lucide.createIcons());

        return { faces, loading, error, status, hasMore, selectedFace, similar,
                 thresholdPct, threshold, loadMore, selectFace, searchSimilar, openFile, onErr };
    }
};
