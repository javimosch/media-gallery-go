const Lightbox = {
    props: {
        file: Object
    },
    emits: ['close'],
    template: `
    <transition name="fade">
        <div v-if="file" class="lightbox-overlay" @click.self="$emit('close')"
             @touchstart="onTouchStart" @touchmove="onTouchMove" @touchend="onTouchEnd">
            <button class="lightbox-close" @click="$emit('close')">
                <i data-lucide="x" class="w-6 h-6"></i>
            </button>
            <img v-if="isImage(file.relpath)"
                 :src="'/api/download/' + file.sha256"
                 :alt="file.relpath"
                 :style="{ transform: 'translateY(' + dragY + 'px)' }"
                 draggable="false">
            <video v-else-if="isVideo(file.relpath)"
                   :src="'/api/stream/' + file.sha256"
                   controls autoplay playsinline
                   :style="{ transform: 'translateY(' + dragY + 'px)' }"
                   class="rounded-lg">
            </video>
            <div v-else class="text-white text-center">
                <p class="mb-4">Preview not available</p>
                <a :href="'/api/download/' + file.sha256"
                   class="px-4 py-2 bg-blue-600 rounded-lg">
                    Download
                </a>
            </div>
            <div class="lightbox-info">
                <div>{{ file.relpath }}</div>
                <div class="text-neutral-400 mt-1">{{ formatSize(file.size) }}</div>
            </div>
        </div>
    </transition>
    `,
    setup(props, { emit }) {
        const dragY = Vue.ref(0);
        let startY = 0;
        let dragging = false;

        function isImage(path) {
            return ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'tif', 'tiff'].includes(path.toLowerCase().split('.').pop());
        }
        function isVideo(path) {
            return ['mp4', 'mov', 'avi', 'mkv', 'wmv'].includes(path.toLowerCase().split('.').pop());
        }
        function formatSize(bytes) {
            if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB';
            return (bytes / 1073741824).toFixed(2) + ' GB';
        }

        function onTouchStart(e) {
            if (e.touches.length === 1) {
                startY = e.touches[0].clientY;
                dragging = true;
            }
        }
        function onTouchMove(e) {
            if (dragging && e.touches.length === 1) {
                dragY.value = Math.max(0, e.touches[0].clientY - startY);
            }
        }
        function onTouchEnd() {
            if (dragY.value > 100) {
                emit('close');
            }
            dragY.value = 0;
            dragging = false;
        }

        // reset on file change
        Vue.watch(() => props.file, () => { dragY.value = 0; dragging = false; });

        Vue.onUpdated(() => lucide.createIcons());
        return { isImage, isVideo, formatSize, dragY, onTouchStart, onTouchMove, onTouchEnd };
    }
};
