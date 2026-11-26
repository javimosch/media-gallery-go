const ThumbGrid = {
    props: {
        files: Array,
        loading: Boolean,
        hasMore: Boolean,
        showNames: Boolean,
        dated: Boolean,     // show each file's date under its thumbnail
        segmented: Boolean, // headed sections per month (files must be sorted)
    },
    emits: ['load-more', 'open-file'],
    template: `
    <div>
        <div v-if="files.length === 0 && !loading" class="text-center text-neutral-600 py-20">
            <i data-lucide="image-off" class="w-12 h-12 mx-auto mb-4 opacity-40"></i>
            <p class="text-sm">No files found</p>
        </div>
        <template v-for="section in sections" :key="section.key">
        <h2 v-if="segmented" class="month-heading">{{ section.label }}</h2>
        <div class="thumb-grid">
            <div v-for="file in section.files" :key="file.sha256 + file.relpath"
                 class="thumb-item"
                 @click="$emit('open-file', file)">
                <img :src="'/api/thumb/' + file.sha256 + '?v=6'"
                     :alt="file.relpath"
                     loading="lazy"
                     decoding="async"
                     draggable="false"
                     @error="onImgError">
                <span v-if="showNames" class="name-caption" :title="file.relpath">{{ file.relpath.split('/').pop() }}</span>
                <span v-if="dated && dateLabel(file)" class="date-caption">{{ dateLabel(file) }}</span>
                <span v-if="isVideo(file.relpath)" class="video-badge">
                    <i data-lucide="play" class="w-3 h-3"></i>
                </span>
                <span v-if="file.source === 'replica'" class="replica-badge" title="Served from replica node">
                    <i data-lucide="hard-drive" class="w-2.5 h-2.5"></i>
                </span>
            </div>
        </div>
        </template>
        <div v-if="loading" class="loading-spinner"></div>
        <div v-if="hasMore && !loading && files.length > 0"
             class="scroll-sentinel"
             ref="sentinel">
            <button v-if="!infiniteScroll" class="load-more-btn" @click="$emit('load-more')">Load more</button>
            <div v-else class="loading-spinner"></div>
        </div>
    </div>
    `,
    setup(props, { emit }) {
        const infiniteScroll = Vue.computed(() => galleryState.settings.infiniteScroll);
        let observer = null;

        const sections = Vue.computed(() => {
            const files = props.files || [];
            if (!props.segmented) return files.length ? [{ key: 'all', files }] : [];
            const out = [];
            for (const f of files) {
                const month = f.month || '';
                let last = out[out.length - 1];
                if (!last || last.key !== month) {
                    // MM-YY, e.g. 08-26
                    last = { key: month, label: month ? month.slice(5, 7) + '-' + month.slice(2, 4) : 'No date', files: [] };
                    out.push(last);
                }
                last.files.push(f);
            }
            return out;
        });

        // DD/MM/YY; "--" for the day when only the month is known
        function dateLabel(f) {
            const d = f.date || (f.month ? f.month + '---' : '');
            return d ? d.slice(8, 10) + '/' + d.slice(5, 7) + '/' + d.slice(2, 4) : '';
        }

        function isVideo(path) {
            return ['mp4', 'mov', 'avi', 'mkv', 'wmv'].includes(path.toLowerCase().split('.').pop());
        }
        function onImgError(e) {
            e.target.style.opacity = '0.2';
        }

        function setupObserver() {
            if (observer) { observer.disconnect(); observer = null; }
            if (!infiniteScroll.value) return;
            const scrollRoot = document.querySelector('.scroll-container');
            observer = new IntersectionObserver((entries) => {
                if (entries[0].isIntersecting && props.hasMore && !props.loading) {
                    emit('load-more');
                }
            }, { root: scrollRoot || null, rootMargin: '400px' });
            Vue.nextTick(() => {
                const el = document.querySelector('.scroll-sentinel');
                if (el) observer.observe(el);
            });
        }

        Vue.onMounted(() => {
            lucide.createIcons();
            setupObserver();
        });

        Vue.onUpdated(() => {
            lucide.createIcons();
            // Re-observe sentinel after DOM updates (new content may have shifted it)
            if (infiniteScroll.value && observer) {
                Vue.nextTick(() => {
                    const el = document.querySelector('.scroll-sentinel');
                    if (el) { observer.disconnect(); observer.observe(el); }
                });
            }
        });

        Vue.onUnmounted(() => {
            if (observer) observer.disconnect();
        });

        Vue.watch(infiniteScroll, () => setupObserver());

        return { sections, dateLabel, isVideo, onImgError, infiniteScroll };
    }
};
