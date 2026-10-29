let ws;
let currentStep = 1;
let currentImageBase64 = null;
let viewMode = 'vec'; // 'orig', 'seg', 'vec'
let lastSVG = "";
let lastSegPNG = "";

// Image Dimensions for Coordinate Mapping
let imgNaturalWidth = 0;
let imgNaturalHeight = 0;

// Interactive Editing State
let activeTool = 'pan'; // 'pan', 'recolor', 'merge', 'erase'
let activeColor = '#3b82f6';
let mergeSource = null;
let canUndo = false;
let canRedo = false;

// Zoom & Pan Viewport Transform
let scale = 1.0;
let panX = 0;
let panY = 0;
let isPanning = false;
let startPanMouseX = 0;
let startPanMouseY = 0;
let startPanStageX = 0;
let startPanStageY = 0;
let isSpacePressed = false;

// Pipeline Configuration State
const state = {
  colors: 6,
  min_pixels: 4,
  reject_aa: true,
  smoothness: 0.50,
  detect_corners: true,
  tolerance: 0.75,
  anti_seam: true,
  transparent_bg: false
};

// Error feedback banner helpers
function showError(msg) {
  hideProgress();
  console.error("[Vectorizador]", msg);
  const banner = document.getElementById('error-banner');
  const text = document.getElementById('error-message');
  if (banner && text) {
    text.textContent = msg;
    banner.style.display = 'flex';
  } else {
    alert(msg);
  }
}

function hideError() {
  const banner = document.getElementById('error-banner');
  if (banner) {
    banner.style.display = 'none';
  }
}

// Initialize WebSocket connection for slider interaction and live editing
function initWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/ws`;

  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    document.getElementById('connection-status').innerHTML = '<span style="color: #10b981;">● Conectado (WebSocket)</span>';
  };

  ws.onclose = () => {
    document.getElementById('connection-status').innerHTML = '<span style="color: #ef4444;">● Desconectado - reconectando...</span>';
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
      break;
    case 'error':
      showError('Error en el motor: ' + msg.error);
      break;
  }
}

function showProgress(stage, percent) {
  const box = document.getElementById('progress-box');
  const label = document.getElementById('progress-label');
  const bar = document.getElementById('progress-bar');
  if (box && label && bar) {
    box.classList.add('active');
    label.textContent = stage;
    bar.style.width = percent + '%';
  }
}

function hideProgress() {
  const box = document.getElementById('progress-box');
  if (box) {
    setTimeout(() => box.classList.remove('active'), 250);
  }
}

// Stepper Navigation
function goToStep(step) {
  currentStep = step;

  document.querySelectorAll('.step-item').forEach(el => {
    const s = parseInt(el.getAttribute('data-step'));
    el.classList.remove('active');
    if (s === step) el.classList.add('active');
    if (s < step) el.classList.add('completed');
  });

  document.querySelectorAll('.stage-panel').forEach(el => {
    el.classList.remove('active');
  });
  const currentPanel = document.getElementById(`stage-panel-${step}`);
  if (currentPanel) {
    currentPanel.classList.add('active');
  }

  // Trigger preview switch matching step
  if (step === 2) {
    setViewMode('seg');
  } else if (step >= 3) {
    setViewMode('vec');
  }

  triggerPipeline();
}

// View Mode Switching ('orig', 'seg', 'vec')
function setViewMode(mode) {
  viewMode = mode;
  ['orig', 'seg', 'vec'].forEach(m => {
    const btn = document.getElementById(`view-mode-${m}`);
    if (btn) {
      if (m === mode) {
        btn.style.background = 'var(--accent)';
        btn.style.color = 'white';
      } else {
        btn.style.background = 'var(--bg-card)';
        btn.style.color = 'var(--text-main)';
      }
    }
  });
  updateWorkspaceView();
}

function updateWorkspaceView() {
  const placeholder = document.getElementById('placeholder-text');
  const imgEl = document.getElementById('img-preview');
  const svgEl = document.getElementById('svg-preview');
  const toolbar = document.getElementById('editor-toolbar');
  const hud = document.getElementById('editor-hud');

  if (!currentImageBase64) {
    if (placeholder) placeholder.style.display = 'block';
    if (imgEl) imgEl.style.display = 'none';
    if (svgEl) svgEl.style.display = 'none';
    if (toolbar) toolbar.style.display = 'none';
    if (hud) hud.style.display = 'none';
    return;
  }

  if (placeholder) placeholder.style.display = 'none';
  if (toolbar) toolbar.style.display = 'flex';
  if (hud) hud.style.display = 'flex';

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
}

function renderPaletteSwatches(colors) {
  const container = document.getElementById('palette-swatches');
  if (!container) return;
  container.innerHTML = '';
  if (!colors || colors.length === 0) return;

  colors.forEach(hex => {
    const swatch = document.createElement('div');
    swatch.className = 'swatch';
    swatch.style.backgroundColor = hex;
    swatch.title = `Seleccionar color ${hex} para edición`;
    if (hex.toLowerCase() === activeColor.toLowerCase()) {
      swatch.classList.add('selected');
    }
    swatch.onclick = () => setActiveColor(hex);
    container.appendChild(swatch);
  });

  if (!activeColor && colors.length > 0) {
    setActiveColor(colors[0]);
  }
}

function setActiveColor(hex) {
  activeColor = hex;
  const swatchEl = document.getElementById('active-color-swatch');
  if (swatchEl) {
    swatchEl.style.backgroundColor = hex;
  }
  document.querySelectorAll('.swatch').forEach(el => {
    if (el.title.includes(hex)) {
      el.classList.add('selected');
    } else {
      el.classList.remove('selected');
    }
  });
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

// Interactive Segment Editor Tools
function setActiveTool(tool) {
  activeTool = tool;
  mergeSource = null;

  ['pan', 'recolor', 'merge', 'erase'].forEach(t => {
    const btn = document.getElementById(`tool-${t}`);
    if (btn) {
      if (t === tool) {
        btn.classList.add('active');
      } else {
        btn.classList.remove('active');
      }
    }
  });

  const stage = document.getElementById('viewport-stage');
  const hudHint = document.getElementById('hud-tool-hint');

  if (tool === 'pan') {
    if (stage) stage.style.cursor = 'grab';
    if (hudHint) hudHint.textContent = 'Modo: Navegación (arrastra para mover lienzo, rueda para zoom)';
  } else if (tool === 'recolor') {
    if (stage) stage.style.cursor = 'crosshair';
    if (hudHint) hudHint.textContent = `Modo: Cubo de Pintura (haz clic en un segmento para aplicar ${activeColor})`;
  } else if (tool === 'merge') {
    if (stage) stage.style.cursor = 'crosshair';
    if (hudHint) hudHint.textContent = 'Modo: Fusión (haz clic en el primer segmento que deseas unir)';
  } else if (tool === 'erase') {
    if (stage) stage.style.cursor = 'crosshair';
    if (hudHint) hudHint.textContent = 'Modo: Borrador a Transparente (haz clic en un segmento para eliminarlo)';
  }
}

function handleStageClick(e) {
  if (activeTool === 'pan' || isPanning) return;

  const pt = getImageCoordinates(e);
  if (!pt) return;

  hideError();

  if (activeTool === 'recolor') {
    showProgress('Recoloreando segmento...', 30);
    sendSegmentAction({
      action: 'edit',
      tool: 'recolor',
      x: pt.x,
      y: pt.y,
      color: activeColor
    });
  } else if (activeTool === 'erase') {
    showProgress('Borrando segmento a transparente...', 30);
    sendSegmentAction({
      action: 'edit',
      tool: 'erase',
      x: pt.x,
      y: pt.y
    });
  } else if (activeTool === 'merge') {
    if (!mergeSource) {
      mergeSource = pt;
      const hudHint = document.getElementById('hud-tool-hint');
      if (hudHint) {
        hudHint.textContent = `Primer segmento fijado en (${pt.x}, ${pt.y}). Haz clic en el segundo segmento contiguo para fusionar.`;
      }
    } else {
      showProgress('Fusionando segmentos...', 30);
      sendSegmentAction({
        action: 'edit',
        tool: 'merge',
        x: mergeSource.x,
        y: mergeSource.y,
        x2: pt.x,
        y2: pt.y
      });
      mergeSource = null;
      setActiveTool('merge');
    }
  }
}

function sendSegmentAction(payload) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(payload));
  } else {
    // Fallback to HTTP endpoint
    fetch('/api/segments/edit', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    .then(r => r.json())
    .then(data => {
      if (!data.success) throw new Error(data.error);
      handleEditResult(data);
    })
    .catch(err => {
      showError('Error en edición: ' + err.message);
    });
  }
}

function handleEditResult(msg) {
  lastSegPNG = msg.segmentation;
  lastSVG = msg.svg;
  canUndo = !!msg.canUndo;
  canRedo = !!msg.canRedo;

  const btnUndo = document.getElementById('btn-undo');
  const btnRedo = document.getElementById('btn-redo');
  if (btnUndo) btnUndo.disabled = !canUndo;
  if (btnRedo) btnRedo.disabled = !canRedo;

  renderPaletteSwatches(msg.palette);
  updateWorkspaceView();
  hideProgress();
}

function triggerUndo() {
  if (!canUndo) return;
  showProgress('Deshaciendo última acción...', 20);
  sendSegmentAction({ action: 'undo' });
}

function triggerRedo() {
  if (!canRedo) return;
  showProgress('Rehaciendo acción...', 20);
  sendSegmentAction({ action: 'redo' });
}

// Zoom & Pan Viewport Logic
function updateStageTransform() {
  const stage = document.getElementById('viewport-stage');
  const zoomText = document.getElementById('zoom-level');
  if (stage) {
    stage.style.transform = `translate(${panX}px, ${panY}px) scale(${scale})`;
  }
  if (zoomText) {
    zoomText.textContent = `${Math.round(scale * 100)}%`;
  }
}

function zoomStep(factor) {
  const previewBox = document.getElementById('preview-box');
  if (!previewBox) return;

  const rect = previewBox.getBoundingClientRect();
  const centerX = rect.width / 2;
  const centerY = rect.height / 2;

  const newScale = Math.min(Math.max(0.1, scale * factor), 25.0);
  panX = centerX - (centerX - panX) * (newScale / scale);
  panY = centerY - (centerY - panY) * (newScale / scale);
  scale = newScale;
  updateStageTransform();
}

function resetZoom() {
  scale = 1.0;
  panX = 0;
  panY = 0;
  updateStageTransform();
}

function getImageCoordinates(e) {
  if (!imgNaturalWidth || !imgNaturalHeight) return null;
  const stage = document.getElementById('viewport-stage');
  if (!stage) return null;

  const rect = stage.getBoundingClientRect();
  const screenX = e.clientX - rect.left;
  const screenY = e.clientY - rect.top;

  const x = Math.floor(screenX / scale);
  const y = Math.floor(screenY / scale);

  if (x < 0 || x >= imgNaturalWidth || y < 0 || y >= imgNaturalHeight) {
    return null;
  }
  return { x, y };
}

// Setup Viewport Drag & Wheel Event Listeners
function setupViewportInteractions() {
  const previewBox = document.getElementById('preview-box');
  const stage = document.getElementById('viewport-stage');
  const hudCoords = document.getElementById('hud-coords');

  if (!previewBox || !stage) return;

  // Wheel Zoom centered at cursor
  previewBox.addEventListener('wheel', (e) => {
    e.preventDefault();
    const rect = previewBox.getBoundingClientRect();
    const mouseX = e.clientX - rect.left;
    const mouseY = e.clientY - rect.top;

    const delta = e.deltaY < 0 ? 1.15 : 0.87;
    const newScale = Math.min(Math.max(0.1, scale * delta), 25.0);

    panX = mouseX - (mouseX - panX) * (newScale / scale);
    panY = mouseY - (mouseY - panY) * (newScale / scale);
    scale = newScale;
    updateStageTransform();
  }, { passive: false });

  // Pan Mouse Events
  previewBox.addEventListener('mousedown', (e) => {
    if (activeTool === 'pan' || isSpacePressed || e.button === 1) {
      isPanning = true;
      startPanMouseX = e.clientX;
      startPanMouseY = e.clientY;
      startPanStageX = panX;
      startPanStageY = panY;
      stage.style.cursor = 'grabbing';
      e.preventDefault();
    }
  });

  window.addEventListener('mousemove', (e) => {
    if (isPanning) {
      panX = startPanStageX + (e.clientX - startPanMouseX);
      panY = startPanStageY + (e.clientY - startPanMouseY);
      updateStageTransform();
      return;
    }

    // Hover Coordinate HUD
    if (hudCoords) {
      const pt = getImageCoordinates(e);
      if (pt) {
        hudCoords.textContent = `X: ${pt.x}, Y: ${pt.y}`;
      }
    }
  });

  window.addEventListener('mouseup', () => {
    if (isPanning) {
      isPanning = false;
      if (stage) {
        stage.style.cursor = activeTool === 'pan' ? 'grab' : 'crosshair';
      }
    }
  });

  stage.addEventListener('click', (e) => {
    handleStageClick(e);
  });

  // Keyboard Shortcuts (Space for Pan, Ctrl+Z, Ctrl+Y)
  window.addEventListener('keydown', (e) => {
    if (e.code === 'Space' && !isSpacePressed && e.target.tagName !== 'INPUT') {
      isSpacePressed = true;
      if (stage) stage.style.cursor = 'grab';
      e.preventDefault();
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z') {
      e.preventDefault();
      if (e.shiftKey) {
        triggerRedo();
      } else {
        triggerUndo();
      }
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'y') {
      e.preventDefault();
      triggerRedo();
    }
  });

  window.addEventListener('keyup', (e) => {
    if (e.code === 'Space') {
      isSpacePressed = false;
      if (stage) {
        stage.style.cursor = activeTool === 'pan' ? 'grab' : 'crosshair';
      }
    }
  });
}

// Upload Handling via HTTP multipart POST
function handleFileSelect(file) {
  if (!file) return;

  hideError();
  const sizeMB = (file.size / (1024 * 1024)).toFixed(2);
  showProgress(`Cargando archivo '${file.name}' (${sizeMB} MB)...`, 15);

  const localUrl = URL.createObjectURL(file);
  currentImageBase64 = localUrl;

  // Measure natural dimensions
  const tempImg = new Image();
  tempImg.onload = () => {
    imgNaturalWidth = tempImg.naturalWidth;
    imgNaturalHeight = tempImg.naturalHeight;
    resetZoom();
  };
  tempImg.src = localUrl;

  updateWorkspaceView();

  const formData = new FormData();
  formData.append('file', file);
  formData.append('config', JSON.stringify(state));

  showProgress(`Enviando '${file.name}' al motor y vectorizando...`, 25);

  fetch('/api/upload', {
    method: 'POST',
    body: formData
  })
  .then(async (response) => {
    let data;
    try {
      data = await response.json();
    } catch (e) {
      throw new Error(`El servidor respondió con código ${response.status} (sin datos JSON válidos)`);
    }

    if (!response.ok || !data || !data.success) {
      const err = (data && data.error) ? data.error : `Error en la subida (código HTTP ${response.status})`;
      throw new Error(err);
    }
    return data;
  })
  .then(data => {
    imgNaturalWidth = data.width || imgNaturalWidth;
    imgNaturalHeight = data.height || imgNaturalHeight;
    lastSegPNG = data.segmentation;
    lastSVG = data.svg;
    canUndo = false;
    canRedo = false;
    renderPaletteSwatches(data.palette);
    updateWorkspaceView();
    hideProgress();
  })
  .catch(err => {
    console.error("Error al subir archivo:", err);
    showError("Error al procesar imagen: " + err.message);
  });
}

// Load sample directly from server testdata
function loadSample(name) {
  hideError();
  showProgress(`Cargando muestra '${name}' en el servidor...`, 15);

  currentImageBase64 = `/api/samples/${encodeURIComponent(name)}`;

  const tempImg = new Image();
  tempImg.onload = () => {
    imgNaturalWidth = tempImg.naturalWidth;
    imgNaturalHeight = tempImg.naturalHeight;
    resetZoom();
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
      throw new Error(`Error HTTP ${response.status}`);
    }

    if (!response.ok || !data || !data.success) {
      const err = (data && data.error) ? data.error : `Error al cargar muestra (HTTP ${response.status})`;
      throw new Error(err);
    }
    return data;
  })
  .then(data => {
    imgNaturalWidth = data.width || imgNaturalWidth;
    imgNaturalHeight = data.height || imgNaturalHeight;
    lastSegPNG = data.segmentation;
    lastSVG = data.svg;
    canUndo = false;
    canRedo = false;
    renderPaletteSwatches(data.palette);
    updateWorkspaceView();
    hideProgress();
  })
  .catch(err => {
    console.error("Error al cargar muestra:", err);
    showError("No se pudo cargar la muestra: " + err.message);
  });
}

// Populate sample buttons from /api/samples
function populateSampleList() {
  fetch('/api/samples')
    .then(r => r.json())
    .then(list => {
      const container = document.getElementById('sample-buttons-container');
      if (!container || !list) return;
      container.innerHTML = '';
      list.forEach(name => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn-sample';
        btn.textContent = name;
        btn.style.margin = '2px';
        btn.onclick = () => loadSample(name);
        container.appendChild(btn);
      });
    })
    .catch(err => console.log('No se pudieron listar muestras:', err));
}

// Export actions
function downloadSVG() {
  if (!lastSVG) {
    showError('No hay ningún gráfico SVG generado aún para descargar.');
    return;
  }
  const blob = new Blob([lastSVG], { type: 'image/svg+xml;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'vectorizado.svg';
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
  populateSampleList();
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
});
