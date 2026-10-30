let ws;
let currentStep = 1;
let currentImageBase64 = null;
let currentFileName = "imagen.png";
let viewMode = 'vec'; // 'orig', 'seg', 'vec'
let lastSVG = "";
let lastSegPNG = "";

// Image Natural Dimensions for Exact Coordinate Mapping
let imgNaturalWidth = 0;
let imgNaturalHeight = 0;

// Interactive Editing State
let activeTool = 'pan'; // 'pan', 'recolor', 'merge', 'erase', 'zap', 'pencil', 'eyedropper'
let previousTool = 'recolor';
let activeColor = '#3b82f6';
let mergeSource = null;
let canUndo = false;
let canRedo = false;

// Pencil Tool & Palette Candidates State
let pencilWidth = 2;
let isDrawingPencil = false;
let pencilPoints = [];
let candidatePalettes = [];

function cyclePencilWidth() {
  const widths = [1, 2, 4, 8];
  const idx = widths.indexOf(pencilWidth);
  pencilWidth = widths[(idx + 1) % widths.length];
  const el = document.getElementById('pencil-width-val');
  if (el) el.textContent = `${pencilWidth}px`;
  const hudHint = document.getElementById('hud-tool-hint');
  if (hudHint && activeTool === 'pencil') {
    hudHint.textContent = `Modo: Lápiz (Grosor: ${pencilWidth}px)`;
  }
}

// Zoom & Pan Viewport Transform
let scale = 1.0;
let panX = 0;
let panY = 0;
let isPanning = false;
let isDraggingStage = false;
let mouseDownPos = null;
let startPanMouseX = 0;
let startPanMouseY = 0;
let startPanStageX = 0;
let startPanStageY = 0;
let isSpacePressed = false;

// Pipeline Configuration State
const state = {
  mode: 'logo_aa',
  quality: 'medium',
  colors: 8,
  min_pixels: 4,
  reject_aa: true,
  smoothness: 0.50,
  detect_corners: true,
  tolerance: 0.75,
  anti_seam: true,
  transparent_bg: false
};

// Canonical Modes & Quality Presets
const CANONICAL_PRESETS = {
  logo_aa: {
    badge: 'Bordes Suaves',
    high: { min_pixels: 2, smoothness: 0.35, tolerance: 0.50, reject_aa: true, detect_corners: true, colors: 8, qual_desc: 'Alta Fidelidad' },
    medium: { min_pixels: 4, smoothness: 0.50, tolerance: 0.75, reject_aa: true, detect_corners: true, colors: 8, qual_desc: 'Recomendada (Balanceada)' },
    low: { min_pixels: 8, smoothness: 0.75, tolerance: 1.25, reject_aa: true, detect_corners: true, colors: 6, qual_desc: 'Baja (Menos Ruido)' }
  },
  logo_sharp: {
    badge: 'Bordes Definidos',
    high: { min_pixels: 1, smoothness: 0.10, tolerance: 0.25, reject_aa: false, detect_corners: true, colors: 8, qual_desc: 'Máxima Precisión' },
    medium: { min_pixels: 2, smoothness: 0.25, tolerance: 0.50, reject_aa: false, detect_corners: true, colors: 8, qual_desc: 'Bordes Nítidos Limpios' },
    low: { min_pixels: 6, smoothness: 0.45, tolerance: 0.85, reject_aa: false, detect_corners: true, colors: 6, qual_desc: 'Geometría Simplificada' }
  },
  photo: {
    badge: 'Tonos Continuos',
    high: { min_pixels: 3, smoothness: 0.30, tolerance: 0.40, reject_aa: false, detect_corners: false, colors: 16, qual_desc: 'Máximo Detalle' },
    medium: { min_pixels: 8, smoothness: 0.50, tolerance: 0.75, reject_aa: false, detect_corners: false, colors: 12, qual_desc: 'Óptima (Curvas Orgánicas)' },
    low: { min_pixels: 20, smoothness: 0.75, tolerance: 1.30, reject_aa: false, detect_corners: false, colors: 8, qual_desc: 'Efecto Póster Suave' }
  }
};

function setCanonicalMode(mode) {
  state.mode = mode;
  applyPresetParameters();
  updatePresetUI();
  triggerPipeline();
}

function setQualityPreset(quality) {
  state.quality = quality;
  applyPresetParameters();
  updatePresetUI();
  triggerPipeline();
}

function applyPresetParameters() {
  const m = CANONICAL_PRESETS[state.mode] || CANONICAL_PRESETS.logo_aa;
  const p = m[state.quality] || m.medium;

  state.min_pixels = p.min_pixels;
  state.smoothness = p.smoothness;
  state.tolerance = p.tolerance;
  state.reject_aa = p.reject_aa;
  state.detect_corners = p.detect_corners;
  state.colors = p.colors;

  syncDOMControls();
}

function syncDOMControls() {
  const elMinPix = document.getElementById('min-pixels-slider');
  const elMinPixVal = document.getElementById('min-pixels-val');
  if (elMinPix) elMinPix.value = state.min_pixels;
  if (elMinPixVal) elMinPixVal.textContent = `${state.min_pixels} px`;

  const elSmooth = document.getElementById('smoothness-slider');
  const elSmoothVal = document.getElementById('smoothness-val');
  if (elSmooth) elSmooth.value = Math.round(state.smoothness * 100);
  if (elSmoothVal) elSmoothVal.textContent = state.smoothness.toFixed(2);

  const elTol = document.getElementById('tolerance-slider');
  const elTolVal = document.getElementById('tolerance-val');
  if (elTol) elTol.value = Math.round(state.tolerance * 100);
  if (elTolVal) elTolVal.textContent = `${state.tolerance.toFixed(2)} px`;

  const elColors = document.getElementById('color-count-slider');
  const elColorsVal = document.getElementById('color-count-val');
  if (elColors) elColors.value = state.colors;
  if (elColorsVal) elColorsVal.textContent = state.colors;

  const elRejectAA = document.getElementById('reject-aa-checkbox');
  if (elRejectAA) elRejectAA.checked = state.reject_aa;

  const elCorners = document.getElementById('detect-corners-checkbox');
  if (elCorners) elCorners.checked = state.detect_corners;
}

function updatePresetUI() {
  ['logo_aa', 'logo_sharp', 'photo'].forEach(m => {
    const btn = document.getElementById(`btn-mode-${m}`);
    if (btn) {
      if (m === state.mode) {
        btn.className = 'mode-card active p-2 rounded-xl border border-indigo-500/50 bg-indigo-950/30 flex flex-col items-center text-center transition hover:border-indigo-400';
      } else {
        btn.className = 'mode-card p-2 rounded-xl border border-white/10 bg-zinc-900/40 hover:bg-white/5 flex flex-col items-center text-center transition hover:border-white/20';
      }
    }
  });

  ['low', 'medium', 'high'].forEach(q => {
    const btn = document.getElementById(`btn-qual-${q}`);
    if (btn) {
      if (q === state.quality) {
        btn.className = 'flex-1 py-1 rounded-lg bg-indigo-600 text-white font-medium shadow-sm transition text-center flex items-center justify-center gap-1.5';
      } else {
        btn.className = 'flex-1 py-1 rounded-lg text-zinc-400 hover:text-zinc-200 transition font-medium text-center flex items-center justify-center gap-1.5';
      }
    }
  });

  const mDef = CANONICAL_PRESETS[state.mode] || CANONICAL_PRESETS.logo_aa;
  const pDef = mDef[state.quality] || mDef.medium;

  const badgeMode = document.getElementById('mode-badge');
  if (badgeMode) badgeMode.textContent = mDef.badge;

  const badgeQual = document.getElementById('quality-badge');
  if (badgeQual) badgeQual.textContent = pDef.qual_desc;
}

// Error feedback banner helpers
function showError(msg) {
  hideProgress();
  console.error("[Vectorizador]", msg);
  const banner = document.getElementById('error-banner');
  const text = document.getElementById('error-message');
  if (banner && text) {
    text.textContent = msg;
    banner.classList.remove('hidden');
    banner.classList.add('flex');
  } else {
    alert(msg);
  }
}

function hideError() {
  const banner = document.getElementById('error-banner');
  if (banner) {
    banner.classList.add('hidden');
    banner.classList.remove('flex');
  }
}

// Quick Export Dropdown in Header
function toggleQuickExportMenu() {
  const menu = document.getElementById('quick-export-menu');
  if (menu) {
    menu.classList.toggle('hidden');
  }
}

function hideQuickExportMenu() {
  const menu = document.getElementById('quick-export-menu');
  if (menu) {
    menu.classList.add('hidden');
  }
}

document.addEventListener('click', (e) => {
  const container = document.getElementById('header-export-container');
  if (container && !container.contains(e.target)) {
    hideQuickExportMenu();
  }
});

// Initialize WebSocket connection for slider interaction and live editing
function initWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/ws`;

  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    const el = document.getElementById('connection-status');
    if (el) {
      el.className = 'hidden lg:flex items-center space-x-2 px-2.5 py-1 rounded-full bg-emerald-950/40 border border-emerald-500/20 text-emerald-400 text-[11px] font-mono';
      el.innerHTML = `
        <span class="relative flex h-2 w-2">
          <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
          <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
        </span>
        <span class="tracking-tight">Motor Conectado</span>
      `;
    }
  };

  ws.onclose = () => {
    const el = document.getElementById('connection-status');
    if (el) {
      el.className = 'hidden lg:flex items-center space-x-2 px-2.5 py-1 rounded-full bg-rose-950/40 border border-rose-500/20 text-rose-400 text-[11px] font-mono';
      el.innerHTML = `
        <span class="relative flex h-2 w-2">
          <span class="relative inline-flex rounded-full h-2 w-2 bg-rose-500"></span>
        </span>
        <span class="tracking-tight">Desconectado (Reintentando...)</span>
      `;
    }
    setTimeout(initWebSocket, 2000);
  };

  ws.onerror = (err) => {
    console.error("Error WebSocket:", err);
  };

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      handleServerMessage(msg);
    } catch (e) {
      console.error("Error al procesar mensaje WebSocket:", e);
    }
  };
}

function handleServerMessage(msg) {
  switch (msg.type) {
    case 'progress':
      showProgress(msg.stage, msg.percent);
      break;
    case 'palette':
      renderPaletteSwatches(msg.colors);
      break;
    case 'segmentation':
      lastSegPNG = msg.preview;
      if (viewMode === 'seg') updateWorkspaceView();
      break;
    case 'svg':
      lastSVG = msg.svg;
      updateWorkspaceView();
      hideProgress();
      break;
    case 'edit_result':
      handleEditResult(msg);
      hideProgress();
      break;
    case 'error':
      showError('Error en el motor: ' + msg.error);
      hideProgress();
      break;
  }
}

let hideProgressTimer = null;

function showProgress(stage, percent) {
  if (hideProgressTimer) {
    clearTimeout(hideProgressTimer);
    hideProgressTimer = null;
  }
  const box = document.getElementById('progress-box');
  const label = document.getElementById('progress-label');
  const pct = document.getElementById('progress-percent');
  const bar = document.getElementById('progress-bar-fill');
  const veil = document.getElementById('canvas-busy-veil');

  if (box && label && bar) {
    box.classList.remove('hidden');
    box.classList.add('flex');
    label.textContent = stage;
    if (pct) pct.textContent = `${percent}%`;
    bar.style.width = `${percent}%`;
  }
  if (veil) {
    veil.classList.remove('hidden');
  }
}

function hideProgress() {
  const box = document.getElementById('progress-box');
  const pct = document.getElementById('progress-percent');
  const bar = document.getElementById('progress-bar-fill');
  const veil = document.getElementById('canvas-busy-veil');

  if (bar) bar.style.width = '100%';
  if (pct) pct.textContent = '100%';

  if (hideProgressTimer) {
    clearTimeout(hideProgressTimer);
  }

  hideProgressTimer = setTimeout(() => {
    if (box) {
      box.classList.add('hidden');
      box.classList.remove('flex');
    }
    if (veil) {
      veil.classList.add('hidden');
    }
    hideProgressTimer = null;
  }, 250);
}

// Desplazamiento de Secciones (Ventana Única)
function scrollToSection(id) {
  const el = document.getElementById(id);
  const container = document.getElementById('sidebar-scroll-container');
  if (el && container) {
    const elTop = el.offsetTop - container.offsetTop;
    container.scrollTo({ top: elTop - 8, behavior: 'smooth' });
  } else if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }
}

function goToStep(step) {
  currentStep = step;
  const map = { 1: 'sec-input', 2: 'sec-palette', 3: 'sec-tuning', 4: 'sec-export' };
  if (map[step]) {
    scrollToSection(map[step]);
  }
}

// View Mode Switching ('orig', 'seg', 'vec')
function setViewMode(mode) {
  viewMode = mode;
  ['orig', 'seg', 'vec'].forEach(m => {
    const btn = document.getElementById(`view-mode-${m}`);
    if (btn) {
      if (m === mode) {
        btn.className = 'px-2.5 py-1 rounded-lg bg-indigo-600 text-white font-medium shadow-sm transition';
      } else {
        btn.className = 'px-2.5 py-1 rounded-lg text-zinc-400 hover:text-zinc-200 transition font-medium';
      }
    }
  });
  updateWorkspaceView();
  hideProgress();
}

function updateWorkspaceView() {
  const imgEl = document.getElementById('img-preview');
  const svgEl = document.getElementById('svg-preview');
  const placeholder = document.getElementById('placeholder-text');

  if (!imgEl || !svgEl) return;

  if (!currentImageBase64) {
    if (placeholder) placeholder.classList.remove('hidden');
    imgEl.style.display = 'none';
    svgEl.style.display = 'none';
    return;
  }

  if (placeholder) placeholder.classList.add('hidden');

  if (viewMode === 'orig') {
    imgEl.src = currentImageBase64;
    imgEl.style.display = 'block';
    svgEl.style.display = 'none';
  } else if (viewMode === 'seg') {
    if (lastSegPNG) {
      imgEl.src = lastSegPNG;
      imgEl.style.display = 'block';
      svgEl.style.display = 'none';
    } else {
      imgEl.src = currentImageBase64;
      imgEl.style.display = 'block';
      svgEl.style.display = 'none';
    }
  } else if (viewMode === 'vec') {
    if (lastSVG) {
      svgEl.innerHTML = lastSVG;
      svgEl.style.display = 'block';
      imgEl.style.display = 'none';
    } else {
      imgEl.src = currentImageBase64;
      imgEl.style.display = 'block';
      svgEl.style.display = 'none';
    }
  }

  updateSamplingCanvas();
}

function renderPaletteSwatches(colors) {
  const container = document.getElementById('palette-swatches');
  if (!container) return;
  container.innerHTML = '';
  if (!colors || colors.length === 0) return;

  colors.forEach(hex => {
    const isTrans = (hex === 'transparent');
    const isActive = (hex.toLowerCase() === activeColor.toLowerCase());

    const item = document.createElement('div');
    item.className = 'flex items-center justify-between p-2 rounded-lg bg-zinc-950/70 border border-white/5 hover:border-white/15 transition cursor-pointer group';
    if (isActive) {
      item.classList.add('border-indigo-500/40', 'bg-indigo-950/20');
    }

    // Micro-Guía Interactiva en Swatch
    item.setAttribute('data-help-title', isTrans ? 'Color Transparente' : `Color ${hex}`);
    item.setAttribute('data-help-desc', isTrans
      ? 'Fondo o perforación transparente. Al seleccionarlo, el Cubo de Pintura perforará las áreas donde hagas clic.'
      : `Muestra de color activo ${hex}. Haz clic para pintar con este tono.`);
    item.setAttribute('data-help-tip', 'Al hacer clic, se convierte de inmediato en tu color de dibujo activo.');

    const left = document.createElement('div');
    left.className = 'flex items-center space-x-2.5 min-w-0';

    const swatch = document.createElement('div');
    swatch.className = 'w-5 h-5 rounded-md shadow-inner border border-black/30 shrink-0';
    if (isTrans) {
      swatch.classList.add('swatch-transparent');
    } else {
      swatch.style.backgroundColor = hex;
    }

    const info = document.createElement('div');
    info.className = 'truncate';
    const hexText = document.createElement('div');
    hexText.className = 'font-mono text-xs text-zinc-200 font-semibold';
    hexText.textContent = isTrans ? '∅ Transparente' : hex.toUpperCase();
    info.appendChild(hexText);

    left.appendChild(swatch);
    left.appendChild(info);

    const right = document.createElement('div');
    right.className = 'flex items-center space-x-1 shrink-0';
    if (isActive) {
      const badge = document.createElement('span');
      badge.className = 'text-[10px] font-mono text-indigo-400 bg-indigo-950/80 px-1.5 py-0.5 rounded border border-indigo-500/20';
      badge.textContent = 'Activo';
      right.appendChild(badge);
    }

    item.appendChild(left);
    item.appendChild(right);

    item.onclick = () => setActiveColor(hex);
    container.appendChild(item);
  });

  if (colors.length > 0) {
    const hasActive = colors.some(c => c.toLowerCase() === activeColor.toLowerCase());
    if (!hasActive) {
      setActiveColor(colors[0]);
    }
  }
}

function setActiveColor(hex) {
  activeColor = hex;
  const swatchEl = document.getElementById('active-color-swatch');
  if (swatchEl) {
    if (hex === 'transparent') {
      swatchEl.style.backgroundColor = 'transparent';
      swatchEl.classList.add('swatch-transparent');
    } else {
      swatchEl.classList.remove('swatch-transparent');
      swatchEl.style.backgroundColor = hex;
    }
  }

  // Re-render swatch items to update 'Activo' badge
  const container = document.getElementById('palette-swatches');
  if (container) {
    const items = container.querySelectorAll('.p-2');
    items.forEach(item => {
      const isTrans = item.querySelector('.swatch-transparent') && hex === 'transparent';
      const text = item.querySelector('.font-mono')?.textContent.toLowerCase();
      if (isTrans || (text && text === hex.toLowerCase())) {
        item.classList.add('border-indigo-500/40', 'bg-indigo-950/20');
      } else {
        item.classList.remove('border-indigo-500/40', 'bg-indigo-950/20');
      }
    });
  }

  const hudHint = document.getElementById('hud-tool-hint');
  if (hudHint && activeTool === 'recolor') {
    hudHint.textContent = hex === 'transparent'
      ? 'Modo: Cubo de Pintura (haz clic en un segmento para hacerlo transparente)'
      : `Modo: Cubo de Pintura (${activeColor})`;
  }
}

// Pipeline Dispatch via WebSocket for live parameter changes
let debounceTimer;
function triggerPipeline() {
  if (!currentImageBase64) return;
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    showError("La conexión con el motor no está lista. Por favor espera la reconexión...");
    return;
  }

  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    hideError();
    showProgress('Actualizando vectorización...', 15);
    ws.send(JSON.stringify({
      action: 'reprocess',
      step: currentStep,
      config: state
    }));
  }, 120);
}

// Selector de Paletas Rápidas (Opción 4: VMDE Quick Palettes)
function renderQuickPalettes(candidates) {
  candidatePalettes = candidates || [];
  const grid = document.getElementById('quick-palettes-grid');
  if (!grid) return;
  grid.innerHTML = '';
  if (!candidates || candidates.length === 0) {
    document.getElementById('quick-palettes-container')?.classList.add('hidden');
    return;
  }
  document.getElementById('quick-palettes-container')?.classList.remove('hidden');

  candidates.forEach(cand => {
    const card = document.createElement('div');
    const isActive = (cand.colors === state.colors);
    card.className = `quick-pal-card p-1.5 rounded-xl border ${isActive ? 'active' : 'border-white/10 bg-zinc-950/60'} flex flex-col items-center justify-between text-center transition gap-1.5`;
    card.id = `quick-pal-${cand.colors}`;

    // Micro-Guía Interactiva en Tarjeta de Paleta
    card.setAttribute('data-help-title', `Paleta de ${cand.colors} Colores`);
    card.setAttribute('data-help-desc', cand.is_optimal
      ? 'Calibración óptima recomendada calculada con la máxima fidelidad cromática.'
      : `Combinación automática calibrada para ${cand.colors} tonos.`);
    card.setAttribute('data-help-tip', 'Haz clic para aplicar esta paleta y reprocesar el vector en 1 paso.');

    // Encabezado / Badge
    const header = document.createElement('div');
    header.className = 'flex items-center justify-between w-full px-0.5';

    const countLabel = document.createElement('span');
    countLabel.className = 'text-[10px] font-mono font-medium text-zinc-300';
    countLabel.textContent = `${cand.colors} col`;
    header.appendChild(countLabel);

    if (cand.is_optimal) {
      const optBadge = document.createElement('span');
      optBadge.className = 'text-[8px] font-mono font-bold text-emerald-400 bg-emerald-950/80 px-1 py-0.2 rounded border border-emerald-500/30';
      optBadge.textContent = '★ Óptima';
      header.appendChild(optBadge);
    }
    card.appendChild(header);

    // Muestras en tira horizontal
    const swatchesRow = document.createElement('div');
    swatchesRow.className = 'flex items-center justify-center gap-0.5 w-full overflow-hidden py-0.5';
    const rawColors = cand.palette || cand.hex_colors || [];
    const distinctColors = Array.from(new Set(rawColors));
    if (distinctColors.length > 0) {
      const displayColors = distinctColors.slice(0, 6);
      displayColors.forEach(hex => {
        const sw = document.createElement('div');
        sw.className = 'h-2.5 flex-1 rounded-sm border border-black/30';
        if (hex === 'transparent') {
          sw.classList.add('swatch-transparent');
        } else {
          sw.style.backgroundColor = hex;
        }
        swatchesRow.appendChild(sw);
      });
    }
    card.appendChild(swatchesRow);

    card.onclick = () => selectQuickPalette(cand);
    grid.appendChild(card);
  });
}

function selectQuickPalette(cand) {
  state.colors = cand.colors;
  const colorSlider = document.getElementById('color-count-slider');
  const colorVal = document.getElementById('color-count-val');
  if (colorSlider) colorSlider.value = state.colors;
  if (colorVal) colorVal.textContent = state.colors;

  updateQuickPalettesActiveCard();
  const rawColors = cand.palette || cand.hex_colors || [];
  if (rawColors.length > 0) {
    renderPaletteSwatches(rawColors);
  }
  triggerPipeline();
}

function updateQuickPalettesActiveCard() {
  const grid = document.getElementById('quick-palettes-grid');
  if (!grid) return;
  const cards = grid.querySelectorAll('.quick-pal-card');
  cards.forEach(c => {
    const candId = `quick-pal-${state.colors}`;
    if (c.id === candId) {
      c.classList.add('active');
      c.classList.remove('border-white/10', 'bg-zinc-950/60');
    } else {
      c.classList.remove('active');
      c.classList.add('border-white/10', 'bg-zinc-950/60');
    }
  });
}

// Color Count Slider Helper (+ / - buttons)
function stepColorSlider(delta) {
  const slider = document.getElementById('color-count-slider');
  const valEl = document.getElementById('color-count-val');
  if (!slider) return;
  let val = parseInt(slider.value) + delta;
  if (val < parseInt(slider.min)) val = parseInt(slider.min);
  if (val > parseInt(slider.max)) val = parseInt(slider.max);
  slider.value = val;
  if (valEl) valEl.textContent = val;
  state.colors = val;
  updateQuickPalettesActiveCard();
  triggerPipeline();
}

// Interactive Segment Editor Tools
function setActiveTool(tool) {
  if (activeTool !== 'eyedropper' && tool === 'eyedropper') {
    previousTool = (activeTool === 'pan') ? 'recolor' : activeTool;
  }
  activeTool = tool;
  mergeSource = null;

  if (tool !== 'pan' && viewMode === 'orig') {
    setViewMode('vec');
  }

  ['pan', 'recolor', 'merge', 'erase', 'zap', 'pencil', 'eyedropper'].forEach(t => {
    const btn = document.getElementById(`tool-${t}`);
    if (btn) {
      if (t === tool) {
        btn.className = 'p-1.5 rounded-lg bg-indigo-600 text-white shadow-sm transition';
      } else {
        btn.className = 'p-1.5 rounded-lg hover:bg-white/10 text-zinc-400 hover:text-zinc-200 transition';
      }
    }
  });

  const stage = document.getElementById('viewport-stage');
  const hudHint = document.getElementById('hud-tool-hint');

  if (stage) {
    stage.className = `viewport-stage tool-${tool}`;
  }

  if (tool === 'pan') {
    if (hudHint) hudHint.textContent = 'Modo: Navegación';
  } else if (tool === 'recolor') {
    if (hudHint) hudHint.textContent = `Modo: Cubo de Pintura (${activeColor})`;
  } else if (tool === 'merge') {
    if (hudHint) hudHint.textContent = 'Modo: Fusión de Segmentos';
  } else if (tool === 'erase') {
    if (hudHint) hudHint.textContent = 'Modo: Borrador a Transparente';
  } else if (tool === 'zap') {
    if (hudHint) hudHint.textContent = 'Modo: Zap (Destruir artefacto)';
  } else if (tool === 'pencil') {
    if (hudHint) hudHint.textContent = `Modo: Lápiz (Grosor: ${pencilWidth}px)`;
  } else if (tool === 'eyedropper') {
    if (hudHint) hudHint.textContent = 'Modo: Cuentagotas (Haz clic en la imagen para absorber color)';
  }
}

// Offscreen Canvas para Muestreo de Color en Alta Fidelidad (Cuentagotas)
const samplingCanvas = document.createElement('canvas');
const samplingCtx = samplingCanvas.getContext('2d', { willReadFrequently: true });

function updateSamplingCanvas() {
  if (!imgNaturalWidth || !imgNaturalHeight) return;
  samplingCanvas.width = imgNaturalWidth;
  samplingCanvas.height = imgNaturalHeight;

  const imgEl = document.getElementById('img-preview');
  if (imgEl && imgEl.complete && imgEl.naturalWidth > 0 && imgEl.src) {
    try {
      samplingCtx.clearRect(0, 0, imgNaturalWidth, imgNaturalHeight);
      samplingCtx.drawImage(imgEl, 0, 0);
    } catch (e) {
      console.warn("No se pudo dibujar en samplingCanvas:", e);
    }
  } else if (currentImageBase64) {
    const temp = new Image();
    temp.onload = () => {
      try {
        samplingCtx.clearRect(0, 0, imgNaturalWidth, imgNaturalHeight);
        samplingCtx.drawImage(temp, 0, 0);
      } catch (e) { }
    };
    temp.src = currentImageBase64;
  }
}

function parseColorToHex(str) {
  if (!str) return null;
  str = str.trim();
  if (str === 'transparent' || str === 'none' || str === 'rgba(0, 0, 0, 0)') return 'transparent';
  if (str.startsWith('#')) {
    if (str.length === 4) {
      return '#' + str[1] + str[1] + str[2] + str[2] + str[3] + str[3];
    }
    return str.substring(0, 7);
  }
  const match = str.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)(?:,\s*([\d.]+))?\)/);
  if (match) {
    const r = parseInt(match[1]);
    const g = parseInt(match[2]);
    const b = parseInt(match[3]);
    const a = match[4] !== undefined ? parseFloat(match[4]) : 1;
    if (a === 0) return 'transparent';
    return '#' + [r, g, b].map(x => {
      const h = x.toString(16);
      return h.length === 1 ? '0' + h : h;
    }).join('');
  }
  return null;
}

function getPixelColorAt(hit, clientX, clientY) {
  // En modo vectorial SVG, inspeccionar primero el nodo SVG bajo el cursor
  if (viewMode === 'vec') {
    const el = document.elementFromPoint(clientX, clientY);
    if (el && el.closest('#svg-preview')) {
      let fill = el.getAttribute('fill') || window.getComputedStyle(el).fill;
      if (fill && fill !== 'none') {
        const hex = parseColorToHex(fill);
        if (hex) return hex;
      }
    }
  }

  // Muestreo del canvas raster subyacente
  if (samplingCtx && hit && hit.x >= 0 && hit.x < imgNaturalWidth && hit.y >= 0 && hit.y < imgNaturalHeight) {
    try {
      const p = samplingCtx.getImageData(hit.x, hit.y, 1, 1).data;
      if (p[3] < 16) return 'transparent';
      return '#' + [p[0], p[1], p[2]].map(x => {
        const h = x.toString(16);
        return h.length === 1 ? '0' + h : h;
      }).join('');
    } catch (err) {
      console.warn("Error al muestrear pixel en canvas:", err);
    }
  }
  return null;
}

// Zoom & Pan Engine with Centered Mouse Transformations
function applyStageTransform() {
  const stage = document.getElementById('viewport-stage');
  if (stage) {
    stage.style.transform = `translate(${panX}px, ${panY}px) scale(${scale})`;
  }
  const zoomIndicator = document.getElementById('zoom-level');
  if (zoomIndicator) {
    zoomIndicator.textContent = `${Math.round(scale * 100)}%`;
  }
  const hudZoom = document.getElementById('hud-zoom');
  if (hudZoom) {
    hudZoom.textContent = `Zoom: ${Math.round(scale * 100)}%`;
  }
}

function zoomStep(factor) {
  const box = document.getElementById('preview-box');
  if (!box) return;
  const rect = box.getBoundingClientRect();
  const centerX = rect.width / 2;
  const centerY = rect.height / 2;
  zoomAtPoint(centerX, centerY, factor);
}

function resetZoom() {
  if (imgNaturalWidth > 0 && imgNaturalHeight > 0) {
    fitStageToScreen();
  } else {
    scale = 1.0;
    panX = 0;
    panY = 0;
    applyStageTransform();
  }
}

function fitStageToScreen() {
  const box = document.getElementById('preview-box');
  if (!box || imgNaturalWidth === 0 || imgNaturalHeight === 0) return;

  const rect = box.getBoundingClientRect();
  const pad = 40;
  const availW = Math.max(100, rect.width - pad * 2);
  const availH = Math.max(100, rect.height - pad * 2);

  const scaleX = availW / imgNaturalWidth;
  const scaleY = availH / imgNaturalHeight;
  scale = Math.min(scaleX, scaleY, 1.0); // Fit completely
  if (scale < 0.05) scale = 0.05;

  panX = (rect.width - imgNaturalWidth * scale) / 2;
  panY = (rect.height - imgNaturalHeight * scale) / 2;
  applyStageTransform();
}

function zoomAtPoint(mouseX, mouseY, factor) {
  const newScale = Math.min(Math.max(scale * factor, 0.05), 30.0);
  if (newScale === scale) return;

  const stageX = (mouseX - panX) / scale;
  const stageY = (mouseY - panY) / scale;

  panX = mouseX - stageX * newScale;
  panY = mouseY - stageY * newScale;
  scale = newScale;

  applyStageTransform();
}

function updateStageDimensions(w, h) {
  imgNaturalWidth = w;
  imgNaturalHeight = h;
  const stage = document.getElementById('viewport-stage');
  if (stage) {
    stage.style.width = `${w}px`;
    stage.style.height = `${h}px`;
  }
  const pencilCanvas = document.getElementById('pencil-overlay-canvas');
  if (pencilCanvas) {
    pencilCanvas.width = w;
    pencilCanvas.height = h;
  }
}

// Convert pointer event coordinates directly to native image pixels using transformed stage bounding box
function getImageCoordinates(e) {
  if (!imgNaturalWidth || !imgNaturalHeight) return null;
  const box = document.getElementById('preview-box');
  const stage = document.getElementById('viewport-stage');
  if (!box || !stage) return null;

  // Strict viewport boundary check: cursor MUST be inside visible canvas area
  const boxRect = box.getBoundingClientRect();
  if (e.clientX < boxRect.left || e.clientX >= boxRect.right ||
    e.clientY < boxRect.top || e.clientY >= boxRect.bottom) {
    return null;
  }

  const rect = stage.getBoundingClientRect();
  const relX = e.clientX - rect.left;
  const relY = e.clientY - rect.top;

  if (relX < 0 || relX >= rect.width || relY < 0 || relY >= rect.height) {
    return null;
  }

  const x = Math.floor((relX / rect.width) * imgNaturalWidth);
  const y = Math.floor((relY / rect.height) * imgNaturalHeight);

  if (x < 0 || x >= imgNaturalWidth || y < 0 || y >= imgNaturalHeight) {
    return null;
  }

  return { x, y, stageX: relX, stageY: relY, clientX: e.clientX, clientY: e.clientY };
}

// Visual pulsating feedback indicator directly at image pixel (x, y)
function showClickIndicator(x, y) {
  const stage = document.getElementById('viewport-stage');
  if (!stage) return;

  const dot = document.createElement('div');
  dot.style.position = 'absolute';
  dot.style.left = `${x}px`;
  dot.style.top = `${y}px`;
  dot.style.width = '10px';
  dot.style.height = '10px';
  dot.style.borderRadius = '50%';
  dot.style.backgroundColor = (activeTool === 'erase' || activeColor === 'transparent') ? '#ffffff' : (activeColor || '#3b82f6');
  dot.style.border = '2px solid #ffffff';
  dot.style.boxShadow = '0 0 10px rgba(255,255,255,0.9)';
  dot.style.pointerEvents = 'none';
  dot.style.transform = 'translate(-50%, -50%) scale(1)';
  dot.style.zIndex = '9999';
  dot.style.transition = 'all 0.35s ease-out';
  stage.appendChild(dot);

  requestAnimationFrame(() => {
    dot.style.transform = 'translate(-50%, -50%) scale(3)';
    dot.style.opacity = '0';
  });
  setTimeout(() => dot.remove(), 400);
}

// Sub-pixel Ripple Animation on click in screen coordinates
function showClickRipple(clientX, clientY) {
  const box = document.getElementById('preview-box');
  if (!box) return;
  const boxRect = box.getBoundingClientRect();
  const ripple = document.createElement('div');
  ripple.className = 'click-ripple';
  ripple.style.left = `${clientX - boxRect.left}px`;
  ripple.style.top = `${clientY - boxRect.top}px`;
  box.appendChild(ripple);
  setTimeout(() => ripple.remove(), 500);
}

// Setup Viewport Event Listeners (Zoom, Pan, and Segment Editing)
function setupViewportInteractions() {
  const box = document.getElementById('preview-box');
  const stage = document.getElementById('viewport-stage');
  const coordsHud = document.getElementById('hud-coords');

  if (!box || !stage) return;

  // Track if interaction originated strictly inside the preview canvas box
  let pointerDownOnCanvas = false;

  // Cancel canvas interaction state if pointerdown originates anywhere outside the preview canvas
  window.addEventListener('pointerdown', (e) => {
    if (!box.contains(e.target)) {
      pointerDownOnCanvas = false;
      mouseDownPos = null;
      isDraggingStage = false;
      isPanning = false;
      if (isDrawingPencil) {
        isDrawingPencil = false;
        pencilPoints = [];
        const canvas = document.getElementById('pencil-overlay-canvas');
        if (canvas) {
          const ctx = canvas.getContext('2d');
          ctx.clearRect(0, 0, canvas.width, canvas.height);
        }
      }
    }
  });

  // Wheel Zoom centered at cursor
  box.addEventListener('wheel', (e) => {
    e.preventDefault();
    const rect = box.getBoundingClientRect();
    const mouseX = e.clientX - rect.left;
    const mouseY = e.clientY - rect.top;
    const factor = e.deltaY < 0 ? 1.15 : 0.85;
    zoomAtPoint(mouseX, mouseY, factor);
  }, { passive: false });

  // Unified Pointerdown on PreviewBox
  box.addEventListener('pointerdown', (e) => {
    if (e.button === 2) return; // Ignore right clicks

    // Ignore clicks on floating overlays and outer panels
    if (e.target.closest('#editor-toolbar') ||
      e.target.closest('#editor-hud') ||
      e.target.closest('#error-banner') ||
      e.target.closest('#progress-box') ||
      e.target.closest('#sample-buttons-container') ||
      e.target.closest('aside') ||
      e.target.closest('header')) {
      pointerDownOnCanvas = false;
      return;
    }

    pointerDownOnCanvas = true;
    mouseDownPos = { x: e.clientX, y: e.clientY };
    isDraggingStage = false;

    // Pencil tool drag stroke starts
    if (activeTool === 'pencil' && !isSpacePressed && e.button === 0) {
      const hit = getImageCoordinates(e);
      if (hit) {
        isDrawingPencil = true;
        pencilPoints = [{ x: hit.x, y: hit.y }];
        const canvas = document.getElementById('pencil-overlay-canvas');
        if (canvas) {
          const ctx = canvas.getContext('2d');
          ctx.fillStyle = (activeColor === 'transparent') ? '#ffffff' : activeColor;
          ctx.beginPath();
          ctx.arc(hit.x, hit.y, Math.max(1, pencilWidth / 2), 0, Math.PI * 2);
          ctx.fill();
        }
        e.preventDefault();
        return;
      }
    }

    // Pan starts if tool is pan, or middle mouse, or spacebar is held
    if (activeTool === 'pan' || isSpacePressed || e.button === 1) {
      isPanning = true;
      startPanMouseX = e.clientX;
      startPanMouseY = e.clientY;
      startPanStageX = panX;
      startPanStageY = panY;
      box.style.cursor = 'grabbing';
      e.preventDefault();
    }
  });

  window.addEventListener('pointermove', (e) => {
    if (isDrawingPencil) {
      const hit = getImageCoordinates(e);
      if (hit && pencilPoints.length > 0) {
        const last = pencilPoints[pencilPoints.length - 1];
        if (hit.x !== last.x || hit.y !== last.y) {
          pencilPoints.push({ x: hit.x, y: hit.y });
          const canvas = document.getElementById('pencil-overlay-canvas');
          if (canvas) {
            const ctx = canvas.getContext('2d');
            ctx.lineWidth = pencilWidth;
            ctx.lineCap = 'round';
            ctx.lineJoin = 'round';
            ctx.strokeStyle = (activeColor === 'transparent') ? '#ffffff' : activeColor;
            ctx.beginPath();
            ctx.moveTo(last.x, last.y);
            ctx.lineTo(hit.x, hit.y);
            ctx.stroke();
          }
        }
      }
      return;
    }

    if (mouseDownPos) {
      const dist = Math.hypot(e.clientX - mouseDownPos.x, e.clientY - mouseDownPos.y);
      if (dist > 4) {
        isDraggingStage = true;
      }
    }

    if (isPanning) {
      const dx = e.clientX - startPanMouseX;
      const dy = e.clientY - startPanMouseY;
      panX = startPanStageX + dx;
      panY = startPanStageY + dy;
      applyStageTransform();
      return;
    }

    // Hover Coordinate HUD
    if (coordsHud) {
      const hit = getImageCoordinates(e);
      if (hit) {
        coordsHud.textContent = `X: ${hit.x}, Y: ${hit.y}`;
      }
    }
  });

  window.addEventListener('pointerup', (e) => {
    // If the click did not start inside the canvas viewport, discard completely
    if (!pointerDownOnCanvas || !mouseDownPos) {
      if (isDrawingPencil) {
        isDrawingPencil = false;
        pencilPoints = [];
        const canvas = document.getElementById('pencil-overlay-canvas');
        if (canvas) {
          const ctx = canvas.getContext('2d');
          ctx.clearRect(0, 0, canvas.width, canvas.height);
        }
      }
      isPanning = false;
      isDraggingStage = false;
      mouseDownPos = null;
      pointerDownOnCanvas = false;
      return;
    }

    pointerDownOnCanvas = false;

    // Strict boundary check: If mouse was released outside box or over any overlay/sidebar, cancel
    if (!box.contains(e.target) ||
      e.target.closest('#editor-toolbar') ||
      e.target.closest('#editor-hud') ||
      e.target.closest('#error-banner') ||
      e.target.closest('#progress-box') ||
      e.target.closest('#sample-buttons-container') ||
      e.target.closest('aside') ||
      e.target.closest('header')) {
      if (isDrawingPencil) {
        isDrawingPencil = false;
        pencilPoints = [];
        const canvas = document.getElementById('pencil-overlay-canvas');
        if (canvas) {
          const ctx = canvas.getContext('2d');
          ctx.clearRect(0, 0, canvas.width, canvas.height);
        }
      }
      isPanning = false;
      isDraggingStage = false;
      mouseDownPos = null;
      return;
    }

    if (isDrawingPencil) {
      isDrawingPencil = false;
      const canvas = document.getElementById('pencil-overlay-canvas');
      if (canvas) {
        const ctx = canvas.getContext('2d');
        ctx.clearRect(0, 0, canvas.width, canvas.height);
      }
      if (pencilPoints.length > 0) {
        showProgress(`Aplicando trazo de lápiz (${pencilPoints.length} pts)...`, 30);
        sendSegmentAction({
          action: 'edit',
          tool: 'pencil',
          stroke_width: pencilWidth,
          color: activeColor === 'transparent' ? '#000000' : activeColor,
          points: pencilPoints
        });
        pencilPoints = [];
      }
      return;
    }

    const wasDragging = isDraggingStage;
    const wasPanning = isPanning;

    isPanning = false;
    isDraggingStage = false;
    mouseDownPos = null;

    box.style.cursor = isSpacePressed ? 'grab' : '';

    // If user dragged to pan, or was using Pan tool, do not trigger edit click
    if (wasDragging || wasPanning || activeTool === 'pan') {
      return;
    }

    // Only left button triggers an edit tool
    if (e.button !== 0) {
      return;
    }

    handleStageClick(e);
  });

  window.addEventListener('keydown', (e) => {
    if (e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT')) {
      return;
    }

    if (e.code === 'Space' && !isSpacePressed) {
      isSpacePressed = true;
      if (box) box.style.cursor = 'grab';
    }

    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z') {
      e.preventDefault();
      if (e.shiftKey) {
        triggerRedo();
      } else {
        triggerUndo();
      }
    } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'y') {
      e.preventDefault();
      triggerRedo();
    } else if (!e.ctrlKey && !e.metaKey && !e.altKey) {
      const k = e.key.toLowerCase();
      if (k === 'h') setActiveTool('pan');
      else if (k === 'b' || k === 'c') setActiveTool('recolor');
      else if (k === 'l' || k === 'm') setActiveTool('merge');
      else if (k === 'e') setActiveTool('erase');
      else if (k === 'z') setActiveTool('zap');
      else if (k === 'p') setActiveTool('pencil');
      else if (k === 'i') setActiveTool('eyedropper');
    }
  });

  window.addEventListener('keyup', (e) => {
    if (e.code === 'Space') {
      isSpacePressed = false;
      if (box) box.style.cursor = '';
    }
  });

  window.addEventListener('resize', () => {
    if (imgNaturalWidth > 0) {
      fitStageToScreen();
    }
  });
}

// Handle Direct Segment Click for Recolor, Merge, Erase, Zap, and Eyedropper
function handleStageClick(e) {
  if (activeTool === 'pan') return;

  const hit = getImageCoordinates(e);
  if (!hit) return;

  hideError();
  showClickIndicator(hit.x, hit.y);
  showClickRipple(e.clientX, e.clientY);

  if (activeTool === 'eyedropper') {
    const sampledColor = getPixelColorAt(hit, e.clientX, e.clientY);
    if (sampledColor) {
      setActiveColor(sampledColor);
      showProgress(`Color ${sampledColor} seleccionado`, 100);
      setTimeout(() => hideProgress(), 650);
    }
    // Regresa automáticamente a la herramienta previa de dibujo
    setActiveTool(previousTool || 'recolor');
    return;
  }

  if (activeTool === 'zap') {
    showProgress(`Destruyendo artefacto en (${hit.x}, ${hit.y})...`, 30);
    sendSegmentAction({
      action: 'edit',
      tool: 'zap',
      x: hit.x,
      y: hit.y
    });
  } else if (activeTool === 'recolor') {
    showProgress(`Recoloreando segmento en (${hit.x}, ${hit.y})...`, 30);
    sendSegmentAction({
      action: 'edit',
      tool: activeColor === 'transparent' ? 'erase' : 'recolor',
      x: hit.x,
      y: hit.y,
      color: activeColor
    });
  } else if (activeTool === 'erase') {
    showProgress(`Borrando segmento en (${hit.x}, ${hit.y})...`, 30);
    sendSegmentAction({
      action: 'edit',
      tool: 'erase',
      x: hit.x,
      y: hit.y
    });
  } else if (activeTool === 'merge') {
    if (!mergeSource) {
      mergeSource = { x: hit.x, y: hit.y };
      const hudHint = document.getElementById('hud-tool-hint');
      if (hudHint) {
        hudHint.textContent = `Segmento 1 fijado en (${hit.x}, ${hit.y}). Haz clic en el segundo contiguo para fusionar.`;
      }
    } else {
      showProgress(`Fusionando segmentos (${mergeSource.x}, ${mergeSource.y}) y (${hit.x}, ${hit.y})...`, 30);
      const src = mergeSource;
      mergeSource = null;
      setActiveTool('merge');
      sendSegmentAction({
        action: 'edit',
        tool: 'merge',
        x: src.x,
        y: src.y,
        x2: hit.x,
        y2: hit.y,
        target_x: hit.x,
        target_y: hit.y
      });
    }
  }
}

// Dispatch segment action via WebSocket or HTTP REST fallback
function sendSegmentAction(payload) {
  hideError();

  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(payload));
  } else {
    fetch('/api/segments/edit', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
      .then(async (res) => {
        const data = await res.json();
        if (!res.ok || !data.success) {
          throw new Error(data.error || 'Error al ejecutar acción de edición');
        }
        return data;
      })
      .then(data => {
        handleEditResult(data);
        hideProgress();
      })
      .catch(err => {
        console.error('Error en edición:', err);
        showError(err.message);
        hideProgress();
      });
  }
}

function triggerUndo() {
  if (!canUndo) return;
  showProgress('Deshaciendo cambio...', 30);
  sendSegmentAction({ action: 'undo', tool: 'undo' });
}

function triggerRedo() {
  if (!canRedo) return;
  showProgress('Rehaciendo cambio...', 30);
  sendSegmentAction({ action: 'redo', tool: 'redo' });
}

function handleEditResult(data) {
  if (data.segmentation) {
    lastSegPNG = data.segmentation;
  }
  if (data.svg) {
    lastSVG = data.svg;
  }
  if (data.palette) {
    renderPaletteSwatches(data.palette);
  }

  canUndo = !!(data.canUndo || data.can_undo);
  canRedo = !!(data.canRedo || data.can_redo);

  const undoBtn = document.getElementById('btn-undo');
  const redoBtn = document.getElementById('btn-redo');
  if (undoBtn) undoBtn.disabled = !canUndo;
  if (redoBtn) redoBtn.disabled = !canRedo;

  updateWorkspaceView();
  hideProgress();
}

// Upload Handling via HTTP multipart POST
function handleFileSelect(file) {
  if (!file) return;

  hideError();
  currentFileName = file.name;
  const sizeMB = (file.size / (1024 * 1024)).toFixed(2);
  showProgress(`Cargando '${file.name}' (${sizeMB} MB)...`, 15);

  const localUrl = URL.createObjectURL(file);
  currentImageBase64 = localUrl;

  const tempImg = new Image();
  tempImg.onload = () => {
    updateStageDimensions(tempImg.naturalWidth, tempImg.naturalHeight);
    fitStageToScreen();
    updateImageInfoCard(file.name, `${tempImg.naturalWidth} × ${tempImg.naturalHeight} px`, `${sizeMB} MB`);
  };
  tempImg.src = localUrl;

  updateWorkspaceView();

  const formData = new FormData();
  formData.append('file', file);
  formData.append('config', JSON.stringify(state));

  fetch('/api/upload', {
    method: 'POST',
    body: formData
  })
    .then(async (response) => {
      let data;
      try {
        data = await response.json();
      } catch (e) {
        throw new Error(`El servidor respondió con código ${response.status}`);
      }

      if (!response.ok || !data || !data.success) {
        const err = (data && data.error) ? data.error : `Error HTTP ${response.status}`;
        throw new Error(err);
      }
      return data;
    })
    .then(data => {
      updateStageDimensions(data.width, data.height);
      fitStageToScreen();
      lastSegPNG = data.segmentation;
      lastSVG = data.svg;
      canUndo = false;
      canRedo = false;
      if (data.mode) state.mode = data.mode;
      if (data.quality) state.quality = data.quality;
      updatePresetUI();
      syncDOMControls();
      if (data.detected_colors) {
        state.colors = data.detected_colors;
        const colorSlider = document.getElementById('color-count-slider');
        const colorVal = document.getElementById('color-count-val');
        if (colorSlider) colorSlider.value = state.colors;
        if (colorVal) colorVal.textContent = state.colors;
      }
      renderPaletteSwatches(data.palette);
      if (data.candidates) {
        renderQuickPalettes(data.candidates);
      }
      updateWorkspaceView();
      hideProgress();
    })
    .catch(err => {
      console.error("Error al subir archivo:", err);
      showError("Error al procesar imagen: " + err.message);
    });
}

function updateImageInfoCard(name, dim, size) {
  const card = document.getElementById('image-info-card');
  const dropzone = document.getElementById('dropzone');
  const nameEl = document.getElementById('image-file-name');
  const detEl = document.getElementById('image-file-details');
  const thumb = document.getElementById('image-thumb-img');

  if (card && dropzone && nameEl && detEl && thumb) {
    nameEl.textContent = name;
    detEl.textContent = `${dim} • ${size}`;
    thumb.src = currentImageBase64;
    card.classList.remove('hidden');
    dropzone.classList.add('hidden');
  }
}

// Load sample directly from server testdata
function loadSample(name) {
  hideError();
  currentFileName = name;
  showProgress(`Cargando muestra '${name}' en el servidor...`, 15);

  currentImageBase64 = `/api/samples/${encodeURIComponent(name)}`;

  const tempImg = new Image();
  tempImg.onload = () => {
    updateStageDimensions(tempImg.naturalWidth, tempImg.naturalHeight);
    fitStageToScreen();
    updateImageInfoCard(name, `${tempImg.naturalWidth} × ${tempImg.naturalHeight} px`, 'Muestra testdata');
  };
  tempImg.src = currentImageBase64;

  updateWorkspaceView();

  fetch(`/api/samples/load?name=${encodeURIComponent(name)}`, {
    method: 'POST'
  })
    .then(async (response) => {
      let data;
      try {
        data = await response.json();
      } catch (e) {
        throw new Error(`El servidor respondió con código ${response.status}`);
      }

      if (!response.ok || !data || !data.success) {
        const err = (data && data.error) ? data.error : `Error HTTP ${response.status}`;
        throw new Error(err);
      }
      return data;
    })
    .then(data => {
      updateStageDimensions(data.width, data.height);
      fitStageToScreen();
      lastSegPNG = data.segmentation;
      lastSVG = data.svg;
      canUndo = false;
      canRedo = false;
      if (data.mode) state.mode = data.mode;
      if (data.quality) state.quality = data.quality;
      updatePresetUI();
      syncDOMControls();
      if (data.detected_colors) {
        state.colors = data.detected_colors;
        const colorSlider = document.getElementById('color-count-slider');
        const colorVal = document.getElementById('color-count-val');
        if (colorSlider) colorSlider.value = state.colors;
        if (colorVal) colorVal.textContent = state.colors;
      }
      renderPaletteSwatches(data.palette);
      if (data.candidates) {
        renderQuickPalettes(data.candidates);
      }
      updateWorkspaceView();
      hideProgress();
    })
    .catch(err => {
      console.error("Error al cargar muestra:", err);
      showError("No se pudo cargar la muestra: " + err.message);
    });
}

function populateSampleList() {
  fetch('/api/samples')
    .then(res => res.json())
    .then(list => {
      const container = document.getElementById('sample-buttons-container');
      if (!container || !list) return;
      container.innerHTML = '';
      list.forEach(name => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'px-2 py-1 rounded-lg bg-zinc-950/70 border border-white/10 hover:border-indigo-500/40 hover:bg-white/5 text-zinc-300 text-xs font-mono transition';
        btn.textContent = name;
        btn.onclick = () => loadSample(name);
        container.appendChild(btn);
      });
    })
    .catch(err => console.log('No se pudieron listar muestras:', err));
}

// Multi-Format Export Actions
function downloadSelectedFormat() {
  const select = document.getElementById('export-format-select');
  const fmt = select ? select.value : 'svg';
  downloadFormat(fmt);
}

function downloadFormat(fmt) {
  if (!lastSVG) {
    showError('No hay ningún resultado vectorizado disponible aún para descargar.');
    return;
  }

  const antiSeam = document.getElementById('anti-seam-checkbox')?.checked ?? true;
  const transparentBg = document.getElementById('transparent-bg-checkbox')?.checked ?? false;

  const baseName = currentFileName ? currentFileName.replace(/\.[^/.]+$/, "") : "vectorizado";

  if (fmt === 'svg' && lastSVG && !transparentBg) {
    const blob = new Blob([lastSVG], { type: 'image/svg+xml;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    triggerBrowserDownload(url, `${baseName}.svg`);
    return;
  }

  // Request fresh export from server endpoint with current settings
  const exportUrl = `/api/export?format=${encodeURIComponent(fmt)}&anti_seam=${antiSeam}&transparent_bg=${transparentBg}`;
  const a = document.createElement('a');
  a.href = exportUrl;
  a.download = `${baseName}.${fmt}`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
}

function downloadSVG() {
  downloadFormat('svg');
}

function triggerBrowserDownload(url, filename) {
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function copySVGToClipboard() {
  if (!lastSVG) {
    showError('No hay ningún gráfico SVG generado aún para copiar.');
    return;
  }
  navigator.clipboard.writeText(lastSVG).then(() => {
    alert('Código SVG copiado al portapapeles!');
  }).catch(err => {
    showError('No se pudo copiar al portapapeles: ' + err);
  });
}

// DOM Setup
window.addEventListener('DOMContentLoaded', () => {
  initWebSocket();
  setupViewportInteractions();

  // Dropzone and File Input
  const dropzone = document.getElementById('dropzone');
  const fileInput = document.getElementById('file-input');

  if (dropzone && fileInput) {
    dropzone.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      fileInput.click();
    });

    fileInput.addEventListener('change', (e) => {
      if (e.target.files && e.target.files.length > 0) {
        handleFileSelect(e.target.files[0]);
      }
    });

    dropzone.addEventListener('dragover', (e) => {
      e.preventDefault();
      dropzone.classList.add('dragover');
    });

    dropzone.addEventListener('dragleave', () => {
      dropzone.classList.remove('dragover');
    });

    dropzone.addEventListener('drop', (e) => {
      e.preventDefault();
      dropzone.classList.remove('dragover');
      if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
        handleFileSelect(e.dataTransfer.files[0]);
      }
    });
  }

  // Slider bindings
  const bindSlider = (id, valId, key, transform) => {
    const el = document.getElementById(id);
    const valEl = document.getElementById(valId);
    if (!el || !valEl) return;
    el.oninput = () => {
      const v = transform ? transform(el.value) : el.value;
      valEl.textContent = typeof v === 'number' && !Number.isInteger(v) ? v.toFixed(2) : v;
      state[key] = v;
      if (key === 'colors') {
        updateQuickPalettesActiveCard();
      }
      triggerPipeline();
    };
  };

  bindSlider('color-count-slider', 'color-count-val', 'colors', v => parseInt(v));
  bindSlider('min-pixels-slider', 'min-pixels-val', 'min_pixels', v => parseInt(v));
  bindSlider('smoothness-slider', 'smoothness-val', 'smoothness', v => parseFloat(v) / 100.0);
  bindSlider('tolerance-slider', 'tolerance-val', 'tolerance', v => parseFloat(v) / 100.0);

  // Checkbox bindings
  const bindCheckbox = (id, key) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.onchange = () => {
      state[key] = el.checked;
      triggerPipeline();
    };
  };

  bindCheckbox('reject-aa-checkbox', 'reject_aa');
  bindCheckbox('detect-corners-checkbox', 'detect_corners');
  bindCheckbox('anti-seam-checkbox', 'anti_seam');
  bindCheckbox('transparent-bg-checkbox', 'transparent_bg');

  updatePresetUI();
  syncDOMControls();
  setupRichTooltips();
});

// Sistema de Micro-Guías y Tooltips Ricos Interactivos
function setupRichTooltips() {
  const tooltip = document.getElementById('rich-tooltip');
  const titleEl = document.getElementById('rich-tooltip-title');
  const badgeEl = document.getElementById('rich-tooltip-badge');
  const descEl = document.getElementById('rich-tooltip-desc');
  const tipContainer = document.getElementById('rich-tooltip-tip-container');
  const tipEl = document.getElementById('rich-tooltip-tip');

  if (!tooltip || !titleEl || !descEl) return;

  let currentTarget = null;
  let showTimer = null;

  function positionTooltip(mouseX, mouseY) {
    const pad = 12;
    const ttWidth = tooltip.offsetWidth || 280;
    const ttHeight = tooltip.offsetHeight || 110;
    const winW = window.innerWidth;
    const winH = window.innerHeight;

    let posX = mouseX + 14;
    let posY = mouseY + 14;

    if (posX + ttWidth > winW - pad) {
      posX = mouseX - ttWidth - 10;
    }
    if (posY + ttHeight > winH - pad) {
      posY = mouseY - ttHeight - 10;
    }
    if (posX < pad) posX = pad;
    if (posY < pad) posY = pad;

    tooltip.style.left = `${posX}px`;
    tooltip.style.top = `${posY}px`;
  }

  function showTooltip(el, mouseX, mouseY) {
    const title = el.getAttribute('data-help-title');
    const desc = el.getAttribute('data-help-desc');
    const shortcut = el.getAttribute('data-help-shortcut');
    const tip = el.getAttribute('data-help-tip');

    if (!title && !desc) return;

    titleEl.textContent = title || '';
    descEl.textContent = desc || '';

    if (shortcut) {
      badgeEl.textContent = shortcut;
      badgeEl.classList.remove('hidden');
    } else {
      badgeEl.classList.add('hidden');
    }

    if (tip) {
      tipEl.textContent = tip;
      tipContainer.classList.remove('hidden');
    } else {
      tipContainer.classList.add('hidden');
    }

    tooltip.classList.remove('hidden');
    positionTooltip(mouseX, mouseY);
    requestAnimationFrame(() => {
      tooltip.classList.add('visible');
    });
  }

  function hideTooltip() {
    clearTimeout(showTimer);
    currentTarget = null;
    tooltip.classList.remove('visible');
    setTimeout(() => {
      if (!currentTarget) {
        tooltip.classList.add('hidden');
      }
    }, 150);
  }

  document.addEventListener('pointerover', (e) => {
    const target = e.target.closest('[data-help-title], [data-help-desc]');
    if (!target) return;
    if (target === currentTarget) return;

    currentTarget = target;
    clearTimeout(showTimer);
    showTimer = setTimeout(() => {
      if (currentTarget === target) {
        showTooltip(target, e.clientX, e.clientY);
      }
    }, 100);
  });

  document.addEventListener('pointermove', (e) => {
    if (currentTarget && !tooltip.classList.contains('hidden')) {
      positionTooltip(e.clientX, e.clientY);
    }
  });

  document.addEventListener('pointerout', (e) => {
    const target = e.target.closest('[data-help-title], [data-help-desc]');
    if (target && target === currentTarget) {
      hideTooltip();
    }
  });

  window.addEventListener('pointerdown', () => {
    hideTooltip();
  });
}
