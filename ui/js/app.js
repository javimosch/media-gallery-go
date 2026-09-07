// Global state shared across components
const defaultSettings = {
    infiniteScroll: window.innerWidth > 1000,
    fastGallery: false,
};

const savedSettings = {};
try {
    const raw = localStorage.getItem('gallerySettings');
    if (raw) Object.assign(savedSettings, JSON.parse(raw));
} catch (e) { /* localStorage blocked by browser — settings won't persist */ }

const galleryState = Vue.reactive({
    currentView: 'gallery',
    lightboxFile: null,
    lightboxFiles: [],
    lightboxIndex: -1,
    settings: Object.assign({}, defaultSettings, savedSettings),
});

// Persist settings to localStorage (best-effort)
Vue.watch(() => galleryState.settings, (val) => {
    try { localStorage.setItem('gallerySettings', JSON.stringify(val)); } catch (e) {}
}, { deep: true });

async function fetchJSON(url) {
    const res = await fetch(url, { cache: 'no-cache' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
}

function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB';
    return (bytes / 1073741824).toFixed(2) + ' GB';
}

function isVideo(relpath) {
    const ext = relpath.toLowerCase().split('.').pop();
    return ['mp4', 'mov', 'avi', 'mkv', 'wmv'].includes(ext);
}

function isImage(relpath) {
    const ext = relpath.toLowerCase().split('.').pop();
    return ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'tif', 'tiff'].includes(ext);
}

function shortName(c) {
    return c.replace('family-media-', '').replace('-sorted', '');
}

function formatYM(ym) {
    if (!ym || ym.length < 7) return ym;
    const m = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
    const idx = parseInt(ym.slice(5,7));
    if (isNaN(idx) || idx < 1 || idx > 12) return ym;
    return m[idx-1] + ' ' + ym.slice(0,4);
}
